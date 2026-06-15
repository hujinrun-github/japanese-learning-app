package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"japanese-learning-app/internal/data"
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
