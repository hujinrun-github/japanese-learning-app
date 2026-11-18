package errata_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/errata"
	"japanese-learning-app/internal/module/user"
)

const editTestSecret = "errata-edit-test"

type editEnv struct {
	db      *sql.DB
	handler http.Handler
	token1  string
	token2  string
}

func newEditEnv(t *testing.T) *editEnv {
	t.Helper()
	db, err := data.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := data.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES
		(1, 'owner@example.com', 'unused'), (2, 'other@example.com', 'unused')`); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	errata.RegisterRoutes(mux, errata.NewStore(db))
	token1, _, err := user.SignToken(1, editTestSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token2, _, err := user.SignToken(2, editTestSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return &editEnv{
		db:      db,
		handler: user.AuthMiddleware(editTestSecret, mux),
		token1:  token1,
		token2:  token2,
	}
}

func (e *editEnv) request(t *testing.T, method, path, auth, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	res := httptest.NewRecorder()
	e.handler.ServeHTTP(res, req)
	if res.Code != want {
		t.Fatalf("%s %s: status %d, want %d; body %s", method, path, res.Code, want, res.Body.String())
	}
	return res
}

func (e *editEnv) insert(t *testing.T, userID int64, module, itemKey, label, category, description, status string) int64 {
	t.Helper()
	var id int64
	err := e.db.QueryRow(`INSERT INTO errata_reports
		(user_id, module, item_key, item_label, category, description, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		userID, module, itemKey, label, category, description, status).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *editEnv) snapshot(t *testing.T, id int64) *errata.Report {
	t.Helper()
	var report errata.Report
	err := e.db.QueryRow(`SELECT id, user_id, module, item_key, item_label, category, description, status
		FROM errata_reports WHERE id = $1`, id).Scan(
		&report.ID, &report.UserID, &report.Module, &report.ItemKey, &report.ItemLabel,
		&report.Category, &report.Description, &report.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return &report
}

func jsonBody(t *testing.T, fields map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func defaultItemKey(module string) string {
	switch module {
	case "word":
		return "7:dictation"
	case "grammar":
		return "3:quiz:12"
	case "lesson":
		return "5:sentence:2"
	case "shadowing":
		return "5:v2:sentence:3"
	case "writing":
		return "9:essay"
	default:
		return "42"
	}
}

// TestLearnerEditErrataRules covers the category-driven field rules plus the
// ownership/status guards shared by every module.
func TestLearnerEditErrataRules(t *testing.T) {
	e := newEditEnv(t)
	longLabel := strings.Repeat("あ", 301)
	longDescription := strings.Repeat("あ", 1001)

	for _, tc := range []struct {
		name      string
		module    string
		itemKey   string
		category  string
		status    string
		owner     int64
		auth      string // "owner", "other", "none"
		path      string // overrides /api/v1/errata/{id}
		body      string
		want      int
		wantLabel string
		wantKey   string
		wantCat   string
		wantDesc  string
	}{
		{"prompt may edit description", "", "", "", "", 0, "", "",
			jsonBody(t, map[string]any{"description": "新的说明"}), http.StatusOK, "", "", "", "新的说明"},
		{"answer may edit label, key and description", "", "", "answer", "", 0, "", "",
			jsonBody(t, map[string]any{"item_label": "新的题面", "item_key": "3:quiz:99", "description": "修正答案"}),
			http.StatusOK, "新的题面", "3:quiz:99", "answer", "修正答案"},
		{"explanation may edit label", "", "", "explanation", "", 0, "", "",
			jsonBody(t, map[string]any{"item_label": "解析摘要"}), http.StatusOK, "解析摘要", "", "explanation", ""},
		{"audio may edit description only", "", "", "audio", "", 0, "", "",
			jsonBody(t, map[string]any{"description": "音频不同步"}), http.StatusOK, "", "", "audio", "音频不同步"},
		{"audio cannot edit label", "", "", "audio", "", 0, "", "",
			jsonBody(t, map[string]any{"item_label": "改写"}), http.StatusBadRequest, "", "", "", ""},
		{"audio cannot edit item key", "", "", "audio", "", 0, "", "",
			jsonBody(t, map[string]any{"item_key": "3:quiz:99"}), http.StatusBadRequest, "", "", "", ""},
		{"other cannot edit label", "", "", "other", "", 0, "", "",
			jsonBody(t, map[string]any{"item_label": "改写"}), http.StatusBadRequest, "", "", "", ""},
		{"other may edit description", "", "", "other", "", 0, "", "",
			jsonBody(t, map[string]any{"description": "补充说明"}), http.StatusOK, "", "", "other", "补充说明"},
		{"switching to audio follows the new rule", "", "", "", "", 0, "", "",
			jsonBody(t, map[string]any{"category": "audio", "description": "改成音频问题"}),
			http.StatusOK, "", "", "audio", "改成音频问题"},
		{"switching to audio rejects label edit", "", "", "", "", 0, "", "",
			jsonBody(t, map[string]any{"category": "audio", "item_label": "改写"}), http.StatusBadRequest, "", "", "", ""},
		{"resolved report is locked", "", "", "", errata.Resolved, 0, "", "",
			jsonBody(t, map[string]any{"description": "改动"}), http.StatusConflict, "", "", "", ""},
		{"another user's report is invisible", "", "", "", "", 2, "", "",
			jsonBody(t, map[string]any{"description": "改动"}), http.StatusNotFound, "", "", "", ""},
		{"sign-in required", "", "", "", "", 0, "none", "",
			jsonBody(t, map[string]any{"description": "改动"}), http.StatusUnauthorized, "", "", "", ""},
		{"missing report", "", "", "", "", 0, "", "/api/v1/errata/999999",
			jsonBody(t, map[string]any{"description": "改动"}), http.StatusNotFound, "", "", "", ""},
		{"invalid id", "", "", "", "", 0, "", "/api/v1/errata/abc",
			jsonBody(t, map[string]any{"description": "改动"}), http.StatusBadRequest, "", "", "", ""},
		{"empty body", "", "", "", "", 0, "", "", `{}`, http.StatusBadRequest, "", "", "", ""},
		{"invalid category", "", "", "", "", 0, "", "",
			jsonBody(t, map[string]any{"category": "unknown"}), http.StatusBadRequest, "", "", "", ""},
		{"blank description", "", "", "", "", 0, "", "",
			jsonBody(t, map[string]any{"description": "   "}), http.StatusBadRequest, "", "", "", ""},
		{"oversized label", "", "", "", "", 0, "", "",
			jsonBody(t, map[string]any{"item_label": longLabel}), http.StatusBadRequest, "", "", "", ""},
		{"oversized description", "", "", "", "", 0, "", "",
			jsonBody(t, map[string]any{"description": longDescription}), http.StatusBadRequest, "", "", "", ""},
		{"item key with illegal characters", "", "", "", "", 0, "", "",
			jsonBody(t, map[string]any{"item_key": "3/../4"}), http.StatusBadRequest, "", "", "", ""},
		{"module, owner and status cannot be rewritten", "", "", "", "", 0, "", "",
			jsonBody(t, map[string]any{"module": "word", "user_id": 2, "status": "resolved", "description": "仍可改描述"}),
			http.StatusOK, "", "", "", "仍可改描述"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			module, itemKey, category, status, owner := tc.module, tc.itemKey, tc.category, tc.status, tc.owner
			if module == "" {
				module = "grammar"
			}
			if itemKey == "" {
				itemKey = defaultItemKey(module)
			}
			if category == "" {
				category = "prompt"
			}
			if status == "" {
				status = errata.Pending
			}
			if owner == 0 {
				owner = 1
			}
			id := e.insert(t, owner, module, itemKey, "原文摘要", category, "原始描述", status)
			path := fmt.Sprintf("/api/v1/errata/%d", id)
			if tc.path != "" {
				path = tc.path
			}
			auth := e.token1
			switch tc.auth {
			case "other":
				auth = e.token2
			case "none":
				auth = ""
			}
			res := e.request(t, http.MethodPatch, path, auth, tc.body, tc.want)

			wantLabel, wantKey, wantCat, wantDesc := tc.wantLabel, tc.wantKey, tc.wantCat, tc.wantDesc
			if wantLabel == "" {
				wantLabel = "原文摘要"
			}
			if wantKey == "" {
				wantKey = itemKey
			}
			if wantCat == "" {
				wantCat = category
			}
			if wantDesc == "" {
				wantDesc = "原始描述"
			}
			if tc.want == http.StatusOK {
				var out struct {
					Data errata.Report `json:"data"`
				}
				if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				if out.Data.ID != id || out.Data.Description != wantDesc {
					t.Fatalf("PATCH response = %+v, want id %d and description %q", out.Data, id, wantDesc)
				}
			}
			got := e.snapshot(t, id)
			if got == nil {
				t.Fatal("report disappeared after PATCH")
			}
			if got.Module != module || got.UserID != owner || got.Status != status {
				t.Fatalf("immutable fields changed: %+v", got)
			}
			if got.ItemLabel != wantLabel || got.ItemKey != wantKey || got.Category != wantCat || got.Description != wantDesc {
				t.Fatalf("unexpected report after PATCH: %+v", got)
			}
		})
	}
}

// TestLearnerEditErrataItemKeyByModule pins the module-specific item_key shapes.
func TestLearnerEditErrataItemKeyByModule(t *testing.T) {
	e := newEditEnv(t)
	for _, tc := range []struct {
		name, module, key string
		want              int
	}{
		{"word dictation", "word", "7:dictation", http.StatusOK},
		{"word recognition", "word", "7:recognition", http.StatusOK},
		{"word plain id", "word", "7", http.StatusOK},
		{"word unknown mode", "word", "7:choice", http.StatusBadRequest},
		{"grammar quiz question", "grammar", "3:quiz:12", http.StatusOK},
		{"grammar plain id", "grammar", "3", http.StatusOK},
		{"grammar incomplete quiz", "grammar", "3:quiz", http.StatusBadRequest},
		{"grammar wrong segment", "grammar", "3:test:12", http.StatusBadRequest},
		{"lesson sentence", "lesson", "5:sentence:2", http.StatusOK},
		{"lesson plain id", "lesson", "5", http.StatusOK},
		{"lesson wrong segment", "lesson", "5:quiz:2", http.StatusBadRequest},
		{"speaking material", "speaking", "4", http.StatusOK},
		{"speaking rejects sentence suffix", "speaking", "4:line:1", http.StatusBadRequest},
		{"shadowing versioned sentence", "shadowing", "5:v2:sentence:3", http.StatusOK},
		{"shadowing plain id", "shadowing", "5", http.StatusOK},
		{"shadowing missing version", "shadowing", "5:sentence:3", http.StatusBadRequest},
		{"shadowing bad version", "shadowing", "5:vx:sentence:3", http.StatusBadRequest},
		{"writing essay", "writing", "9:essay", http.StatusOK},
		{"writing sentence", "writing", "9:sentence", http.StatusOK},
		{"writing missing mode", "writing", "9", http.StatusBadRequest},
		{"translation sentence", "translation", "42", http.StatusOK},
		{"translation rejects suffix", "translation", "42:1", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initial := defaultItemKey(tc.module)
			id := e.insert(t, 1, tc.module, initial, "原文摘要", "prompt", "原始描述", errata.Pending)
			e.request(t, http.MethodPatch, fmt.Sprintf("/api/v1/errata/%d", id), e.token1,
				jsonBody(t, map[string]any{"item_key": tc.key}), tc.want)
			wantKey := tc.key
			if tc.want != http.StatusOK {
				wantKey = initial
			}
			if got := e.snapshot(t, id); got.ItemKey != wantKey {
				t.Fatalf("item_key = %q, want %q", got.ItemKey, wantKey)
			}
		})
	}
}

// TestLearnerDeleteErrata covers the category-driven deletion policy.
func TestLearnerDeleteErrata(t *testing.T) {
	e := newEditEnv(t)
	for _, tc := range []struct {
		name, module, category, status string
		owner                         int64
		auth, path                    string
		want                          int
		gone                          bool
	}{
		{"word prompt is deletable", "word", "prompt", "", 1, "", "", http.StatusNoContent, true},
		{"grammar answer is deletable", "grammar", "answer", "", 1, "", "", http.StatusNoContent, true},
		{"lesson explanation is deletable", "lesson", "explanation", "", 1, "", "", http.StatusNoContent, true},
		{"shadowing other is deletable", "shadowing", "other", "", 1, "", "", http.StatusNoContent, true},
		{"audio cannot be withdrawn by learner", "shadowing", "audio", "", 1, "", "", http.StatusConflict, false},
		{"speaking audio cannot be withdrawn", "speaking", "audio", "", 1, "", "", http.StatusConflict, false},
		{"resolved report is locked", "writing", "prompt", errata.Resolved, 1, "", "", http.StatusConflict, false},
		{"another user's report is invisible", "translation", "prompt", "", 2, "", "", http.StatusNotFound, false},
		{"sign-in required", "translation", "prompt", "", 1, "none", "", http.StatusUnauthorized, false},
		{"missing report", "translation", "prompt", "", 1, "", "/api/v1/errata/999999", http.StatusNotFound, false},
		{"invalid id", "translation", "prompt", "", 1, "", "/api/v1/errata/abc", http.StatusBadRequest, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := tc.status
			if status == "" {
				status = errata.Pending
			}
			id := e.insert(t, tc.owner, tc.module, defaultItemKey(tc.module), "原文摘要", tc.category, "原始描述", status)
			path := fmt.Sprintf("/api/v1/errata/%d", id)
			if tc.path != "" {
				path = tc.path
			}
			auth := e.token1
			if tc.auth == "none" {
				auth = ""
			}
			e.request(t, http.MethodDelete, path, auth, "", tc.want)
			got := e.snapshot(t, id)
			if tc.gone && got != nil {
				t.Fatalf("report %d still present after DELETE", id)
			}
			if !tc.gone && got == nil {
				t.Fatalf("report %d disappeared after rejected DELETE", id)
			}
		})
	}
}
