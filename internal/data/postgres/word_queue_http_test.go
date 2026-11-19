package postgres

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
)

// Exercise the same authenticated queue used by the home review button, with
// real records: unseen words must never fill a review round.
func TestWordQueueSeparatesReviewFromNewWords(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	users := newPostgresUserStoreForDB(db)
	u, err := users.CreateUser(user.User{Email: "queue-scope@example.test", JLPTLevels: []string{"N5"}}, "unused")
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.CreateUser(user.User{Email: "other-queue@example.test", JLPTLevels: []string{"N5"}}, "unused")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	ids := make([]int64, 8)
	for i := range ids {
		level := "N5"
		if i == 3 {
			level = "N2"
		}
		if err := db.QueryRow(`INSERT INTO words (kanji_form, reading, meaning, jlpt_level) VALUES ($1, 'ご', '词', $2) RETURNING id`, fmt.Sprintf("語%d", i), level).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	// Four due words (including N2), one future word, one reviewed today,
	// one unseen word, and one word only the other user has studied.
	for i := 0; i < 6; i++ {
		next, updated := now.Add(-time.Duration(24-i)*time.Hour), now.Add(-48*time.Hour)
		if i == 4 {
			next = now.Add(24 * time.Hour)
		}
		if i == 5 {
			updated = now.Add(-time.Hour)
		}
		if _, err := db.Exec(`INSERT INTO word_records (user_id, word_id, next_review_at, updated_at) VALUES ($1, $2, $3, $4)`, u.ID, ids[i], next, updated); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO word_records (user_id, word_id, next_review_at, updated_at) VALUES ($1, $2, $3, $4)`, other.ID, ids[7], now.Add(-time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	protected := http.NewServeMux()
	word.NewWordHandler(word.NewWordService(newPostgresWordStoreForDB(db))).RegisterRoutes(protected)
	token, _, err := user.SignToken(u.ID, "queue-test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := user.AuthMiddleware("queue-test", protected)
	for _, tt := range []struct {
		name, query string
		want        []int64
		newWords    bool
		status      int
	}{
		{"default review never includes new words", "?level=N5&limit=4", ids[:3], false, 200},
		{"home review covers all levels without a half-round cap", "?level=all&limit=4", ids[:4], false, 200},
		{"review limit still applies", "?level=all&limit=2", ids[:2], false, 200},
		{"only explicitly requested new words", "?level=N5&queue=new&limit=4", ids[6:], true, 200},
		{"no due words does not fall back to new words", "?level=N4", nil, false, 200},
		{"invalid queue is rejected", "?queue=unknown", nil, false, 400},
		{"new words require a level", "?queue=new&level=all", nil, false, 400},
		{"invalid level is rejected", "?level=invalid", nil, false, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/words/queue"+tt.query, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tt.status, rec.Body.String())
			}
			if tt.status != 200 {
				return
			}
			var envelope struct {
				Data []word.WordCard `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			var got []int64
			for _, card := range envelope.Data {
				got = append(got, card.Word.ID)
				if card.IsNew != tt.newWords {
					t.Errorf("word=%d is_new=%v", card.Word.ID, card.IsNew)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("word IDs=%v want=%v", got, tt.want)
			}
		})
	}
	stats, err := users.GetStats(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.ModuleStats["word"].DueCount != 4 {
		t.Errorf("home due count=%d want=4, matching all-level review", stats.ModuleStats["word"].DueCount)
	}
}
