package errata_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/admin"
	"japanese-learning-app/internal/module/errata"
	"japanese-learning-app/internal/module/user"
)

func TestErrataWorkflow(t *testing.T) {
	db, err := data.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := data.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash)
		VALUES (1, 'learner@example.com', 'unused')`); err != nil {
		t.Fatal(err)
	}
	store := errata.NewStore(db)
	const secret = "errata-test"
	const adminToken = "errata-admin"
	mux := http.NewServeMux()
	errata.RegisterRoutes(mux, store)
	learner := user.AuthMiddleware(secret, mux)
	adminAPI := admin.NewHandler(admin.HandlerConfig{AdminToken: adminToken, ErrataStore: store}).RegisterRoutes()
	token, _, err := user.SignToken(1, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request := func(handler http.Handler, method, path, auth, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: status %d, want %d; body %s", method, path, res.Code, status, res.Body)
		}
		return res
	}

	const report = `{"module":"translation","item_key":"42","item_label":"明日は休みです。","category":"answer","description":"译文里的时间写错了。"}`
	request(learner, "POST", "/api/v1/errata", "", report, http.StatusUnauthorized)
	for _, tc := range []struct{ name, from, to string }{
		{"invalid module", "translation", "unknown"},
		{"invalid category", "answer", "unknown"},
		{"empty description", "译文里的时间写错了。", "  "},
		{"invalid item key", "42", "42/../43"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request(learner, "POST", "/api/v1/errata", token, strings.Replace(report, tc.from, tc.to, 1), http.StatusBadRequest)
		})
	}
	request(learner, "POST", "/api/v1/errata", token, report, http.StatusCreated)
	request(adminAPI, "GET", "/api/admin/errata", token, "", http.StatusUnauthorized)
	list := func(status string) []errata.Report {
		t.Helper()
		res := request(adminAPI, "GET", "/api/admin/errata?status="+status, adminToken, "", http.StatusOK)
		var page struct {
			Items []errata.Report `json:"items"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page.Items
	}
	pending := list("pending")
	if len(pending) != 1 || pending[0].ItemKey != "42" || pending[0].UserID != 1 || pending[0].Description != "译文里的时间写错了。" {
		t.Fatalf("unexpected pending reports: %+v", pending)
	}
	request(adminAPI, "PATCH", "/api/admin/errata/1", adminToken, `{"status":"resolved"}`, http.StatusNoContent)
	if len(list("pending")) != 0 || len(list("resolved")) != 1 {
		t.Fatal("report did not move to resolved")
	}
	request(adminAPI, "PATCH", "/api/admin/errata/1", adminToken, `{"status":"pending"}`, http.StatusNoContent)
	if len(list("pending")) != 1 {
		t.Fatal("report did not reopen")
	}
	request(adminAPI, "PATCH", "/api/admin/errata/99", adminToken, `{"status":"resolved"}`, http.StatusNotFound)
}

func TestLearnerErrataHistory(t *testing.T) {
	db, err := data.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := data.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES
		(1, 'learner@example.com', 'unused'), (2, 'other@example.com', 'unused'), (3, 'empty@example.com', 'unused')`); err != nil {
		t.Fatal(err)
	}

	store := errata.NewStore(db)
	mux := http.NewServeMux()
	errata.RegisterRoutes(mux, store)
	const secret = "errata-history-test"
	handler := user.AuthMiddleware(secret, mux)
	for i := 1; i <= 22; i++ {
		userID := int64(1)
		if i == 22 {
			userID = 2
		}
		id, err := store.Save(t.Context(), errata.Report{
			UserID: userID, Module: "writing", ItemKey: "weekend:essay",
			ItemLabel: "我的周末", Category: "prompt", Description: fmt.Sprintf("问题 %d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if i == 21 {
			updated, err := store.SetStatus(t.Context(), id, errata.Resolved)
			if err != nil || !updated {
				t.Fatalf("mark resolved: updated %v, err %v", updated, err)
			}
		}
	}

	for _, tc := range []struct {
		name, query     string
		userID          int64
		status, count   int
		firstID, lastID int64
		firstStatus     string
		hasMore         bool
	}{
		{"first page is newest and scoped to the signed-in user", "?user_id=2", 1, http.StatusOK, 20, 21, 2, errata.Resolved, true},
		{"second page", "?page=2", 1, http.StatusOK, 1, 1, 1, errata.Pending, false},
		{"other learner", "", 2, http.StatusOK, 1, 22, 22, errata.Pending, false},
		{"empty history", "", 3, http.StatusOK, 0, 0, 0, "", false},
		{"invalid page", "?page=0", 1, http.StatusBadRequest, 0, 0, 0, "", false},
		{"requires sign-in", "", 0, http.StatusUnauthorized, 0, 0, 0, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/errata"+tc.query, nil)
			if tc.userID > 0 {
				token, _, err := user.SignToken(tc.userID, secret, time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != tc.status {
				t.Fatalf("status = %d, want %d; body = %s", resp.Code, tc.status, resp.Body)
			}
			if tc.status != http.StatusOK {
				return
			}
			var result struct {
				Data struct {
					Items   []errata.Report `json:"items"`
					HasMore bool            `json:"has_more"`
				} `json:"data"`
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			items := result.Data.Items
			if items == nil || len(items) != tc.count || result.Data.HasMore != tc.hasMore {
				t.Fatalf("got %d reports, more = %v; want %d reports, more = %v", len(items), result.Data.HasMore, tc.count, tc.hasMore)
			}
			for _, item := range items {
				if item.UserID != tc.userID {
					t.Fatalf("report %d belongs to %d, want %d", item.ID, item.UserID, tc.userID)
				}
			}
			if len(items) > 0 && (items[0].ID != tc.firstID || items[len(items)-1].ID != tc.lastID || items[0].Status != tc.firstStatus) {
				t.Fatalf("unexpected report order or status: %+v", items)
			}
		})
	}
}
