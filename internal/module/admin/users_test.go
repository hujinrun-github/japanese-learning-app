package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/user"
)

func TestDeleteUserRemovesUserAndOwnedData(t *testing.T) {
	db, store, srv := newUsersTestServer(t)
	defer srv.Close()

	created, err := store.Create("Delete Me", "delete-user@example.com", "hash", `["N5"]`)
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}
	seedUserOwnedData(t, db, created.ID)

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/admin/users/%d", srv.URL, created.ID), nil)
	if err != nil {
		t.Fatalf("NewRequest DELETE: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("DELETE user: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("DELETE user status = %d, want %d, body: %s", resp.StatusCode, http.StatusNoContent, body)
	}

	if _, err := store.GetByID(created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetByID after delete error = %v, want sql.ErrNoRows", err)
	}
	assertCount(t, db, "SELECT COUNT(*) FROM notes WHERE user_id = ?", created.ID, 0)
	assertCount(t, db, "SELECT COUNT(*) FROM note_links WHERE user_id = ?", created.ID, 0)
	assertCount(t, db, "SELECT COUNT(*) FROM translation_records WHERE user_id = ?", created.ID, 0)
}

func TestUpdateUserPassword(t *testing.T) {
	_, store, srv := newUsersTestServer(t)
	defer srv.Close()

	created, err := store.Create("Password User", "password-user@example.com", user.HashPassword("old-password"), `["N5"]`)
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}

	tests := []struct {
		name       string
		userID     int64
		body       map[string]string
		wantStatus int
		wantLogin  bool
	}{
		{
			name:       "updates password",
			userID:     created.ID,
			body:       map[string]string{"new_password": "new-password"},
			wantStatus: http.StatusNoContent,
			wantLogin:  true,
		},
		{
			name:       "rejects empty password",
			userID:     created.ID,
			body:       map[string]string{"new_password": ""},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns not found for missing user",
			userID:     created.ID + 999,
			body:       map[string]string{"new_password": "new-password"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("Marshal body: %v", err)
			}
			req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/admin/users/%d/password", srv.URL, tt.userID), bytes.NewReader(body))
			if err != nil {
				t.Fatalf("NewRequest PUT password: %v", err)
			}
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")

			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("PUT password: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("PUT password status = %d, want %d, body: %s", resp.StatusCode, tt.wantStatus, respBody)
			}

			if tt.wantLogin {
				adapter := data.NewUserStoreAdapter(store)
				svc := user.NewUserService(adapter, "test-secret", &user.StubMailer{}, "http://localhost")
				if _, err := svc.Login(user.LoginReq{Email: created.Email, Password: tt.body["new_password"]}); err != nil {
					t.Fatalf("Login with new password: %v", err)
				}
			}
		})
	}
}

func TestUsersHandlerAcceptsAdminUserStoreInterface(t *testing.T) {
	fakeStore := &fakeAdminUserStore{
		users: []user.User{{ID: 1, Name: "Postgres User", Email: "pg-user@example.com", JLPTLevels: []string{"N5"}}},
	}
	h := NewHandler(HandlerConfig{
		AdminToken: "test-token",
		UserStore:  fakeStore,
	})
	srv := httptest.NewServer(h.RegisterRoutes())
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/admin/users", nil)
	if err != nil {
		t.Fatalf("NewRequest GET users: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET users: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET users status = %d, want %d, body: %s", resp.StatusCode, http.StatusOK, body)
	}
}

func newUsersTestServer(t *testing.T) (*sql.DB, *data.UserStore, *httptest.Server) {
	t.Helper()

	dbPath := t.TempDir() + "/test.db"
	db, err := data.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := data.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	store := data.NewUserStore(db)
	h := NewHandler(HandlerConfig{
		AdminToken: "test-token",
		UserStore:  store,
		DB:         db,
	})
	return db, store, httptest.NewServer(h.RegisterRoutes())
}

type fakeAdminUserStore struct {
	users []user.User
}

func (s *fakeAdminUserStore) ListAllUsers(offset, limit int) ([]user.User, int, error) {
	return s.users, len(s.users), nil
}

func (s *fakeAdminUserStore) GetStats(userID int64) (*user.UserStats, error) {
	return &user.UserStats{}, nil
}

func (s *fakeAdminUserStore) DeleteUser(id int64) error {
	return nil
}

func (s *fakeAdminUserStore) UpdatePassword(userID int64, newPasswordHash string) error {
	return nil
}

func seedUserOwnedData(t *testing.T, db *sql.DB, userID int64) {
	t.Helper()

	firstNoteID := insertNote(t, db, userID, "first note")
	secondNoteID := insertNote(t, db, userID, "second note")
	if _, err := db.Exec(
		`INSERT INTO note_links (user_id, note_id, target_note_id, relation) VALUES (?, ?, ?, 'related')`,
		userID, firstNoteID, secondNoteID,
	); err != nil {
		t.Fatalf("insert note link: %v", err)
	}

	sourceRes, err := db.Exec(
		`INSERT INTO translation_sources (title, source_type, raw_content) VALUES ('source', 'manual', 'raw')`,
	)
	if err != nil {
		t.Fatalf("insert translation source: %v", err)
	}
	sourceID, err := sourceRes.LastInsertId()
	if err != nil {
		t.Fatalf("translation source id: %v", err)
	}
	sentenceRes, err := db.Exec(
		`INSERT INTO translation_sentences (source_id, direction, source_text, reference_translation) VALUES (?, 'jp2cn', '日本語', 'Japanese')`,
		sourceID,
	)
	if err != nil {
		t.Fatalf("insert translation sentence: %v", err)
	}
	sentenceID, err := sentenceRes.LastInsertId()
	if err != nil {
		t.Fatalf("translation sentence id: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO translation_records (user_id, sentence_id, user_translation) VALUES (?, ?, '中文')`,
		userID, sentenceID,
	); err != nil {
		t.Fatalf("insert translation record: %v", err)
	}
}

func insertNote(t *testing.T, db *sql.DB, userID int64, title string) int64 {
	t.Helper()

	res, err := db.Exec(
		`INSERT INTO notes (user_id, type, title, content) VALUES (?, 'word', ?, 'content')`,
		userID, title,
	)
	if err != nil {
		t.Fatalf("insert note: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("note id: %v", err)
	}
	return id
}

func assertCount(t *testing.T, db *sql.DB, query string, userID int64, want int) {
	t.Helper()

	var got int
	if err := db.QueryRow(query, userID).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("count query %q = %d, want %d", query, got, want)
	}
}
