package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"japanese-learning-app/internal/data"
)

func TestAuthMiddleware(t *testing.T) {
	cfg := HandlerConfig{AdminToken: "test-token"}
	h := NewHandler(cfg)

	handler := h.auth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"wrong token", "wrong", http.StatusUnauthorized},
		{"correct token", "test-token", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/admin/test", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			rec := httptest.NewRecorder()
			handler(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("got %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestWordCRUD(t *testing.T) {
	// Set up test SQLite database
	dbPath := t.TempDir() + "/test.db"
	db, err := data.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	if err := data.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	cfg := HandlerConfig{
		AdminToken: "test-token",
		WordStore:  data.NewWordStore(db),
		DB:         db,
	}
	h := NewHandler(cfg)
	mux := h.RegisterRoutes()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	authHeader := func() string { return "Bearer test-token" }
	client := srv.Client()

	// ---- Test CREATE ----
	createBody := `{"kanji_form":"試みる","reading":"こころみる","meaning":"尝试","part_of_speech":"verb","jlpt_level":"N1","examples":[{"japanese":"新しい方法を試みる","chinese":"尝试新方法"}]}`
	req, err := http.NewRequest("POST", srv.URL+"/api/admin/words", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("NewRequest POST: %v", err)
	}
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST: got %d, want %d, body: %s", resp.StatusCode, http.StatusCreated, body)
	}
	resp.Body.Close()

	// ---- Test LIST (find the word we just created by search) ----
	req, err = http.NewRequest("GET", srv.URL+"/api/admin/words?search=試みる", nil)
	if err != nil {
		t.Fatalf("NewRequest GET: %v", err)
	}
	req.Header.Set("Authorization", authHeader())
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var listResp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		resp.Body.Close()
		t.Fatalf("GET decode: %v", err)
	}
	resp.Body.Close()
	if listResp.Total < 1 {
		t.Fatalf("total: got %d, want at least 1", listResp.Total)
	}

	// Find the word we created by kanji_form
	var id int64
	for _, item := range listResp.Items {
		if item["kanji_form"] == "試みる" {
			id = int64(item["id"].(float64))
			break
		}
	}
	if id == 0 {
		t.Fatal("created word not found in list")
	}

	// ---- Test UPDATE ----
	updateBody := fmt.Sprintf(`{"kanji_form":"試みる","reading":"こころみる","meaning":"试图","part_of_speech":"verb","jlpt_level":"N1","examples":[{"japanese":"新しい方法を試みる","chinese":"尝试新方法"},{"japanese":"解決を試みる","chinese":"试图解决"}]}`)
	req, err = http.NewRequest("PUT", fmt.Sprintf("%s/api/admin/words/%d", srv.URL, id), strings.NewReader(updateBody))
	if err != nil {
		t.Fatalf("NewRequest PUT: %v", err)
	}
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("PUT: got %d, want %d, body: %s", resp.StatusCode, http.StatusOK, body)
	}
	resp.Body.Close()

	// Verify the update by fetching the list again and checking meaning changed
	req, _ = http.NewRequest("GET", srv.URL+"/api/admin/words?search=試みる", nil)
	req.Header.Set("Authorization", authHeader())
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET after update: %v", err)
	}
	var verifyResp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	json.NewDecoder(resp.Body).Decode(&verifyResp)
	resp.Body.Close()
	found := false
	for _, item := range verifyResp.Items {
		if int64(item["id"].(float64)) == id {
			found = true
			if item["meaning"] != "试图" {
				t.Errorf("meaning after update = %q, want %q", item["meaning"], "试图")
			}
			break
		}
	}
	if !found {
		t.Error("updated word not found in search results")
	}

	// ---- Test DELETE ----
	req, err = http.NewRequest("DELETE", fmt.Sprintf("%s/api/admin/words/%d", srv.URL, id), nil)
	if err != nil {
		t.Fatalf("NewRequest DELETE: %v", err)
	}
	req.Header.Set("Authorization", authHeader())
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("DELETE: got %d, want %d, body: %s", resp.StatusCode, http.StatusNoContent, body)
	}
	resp.Body.Close()

	// Verify the word is gone
	req, _ = http.NewRequest("GET", srv.URL+"/api/admin/words?search=試みる", nil)
	req.Header.Set("Authorization", authHeader())
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET after delete: %v", err)
	}
	var afterDelete struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	json.NewDecoder(resp.Body).Decode(&afterDelete)
	resp.Body.Close()
	for _, item := range afterDelete.Items {
		if item["kanji_form"] == "試みる" {
			t.Error("word still exists after delete")
		}
	}

	// ---- Test CREATE with missing required fields ----
	badBody := `{"kanji_form":"忘れ物"}`
	req, _ = http.NewRequest("POST", srv.URL+"/api/admin/words", strings.NewReader(badBody))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST bad: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Errorf("POST with missing fields: got %d, want %d, body: %s", resp.StatusCode, http.StatusBadRequest, body)
	}
	resp.Body.Close()

	// ---- Test auth rejection for each endpoint ----
	authTests := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", srv.URL + "/api/admin/words", ""},
		{"POST", srv.URL + "/api/admin/words", createBody},
		{"PUT", srv.URL + "/api/admin/words/1", updateBody},
		{"DELETE", srv.URL + "/api/admin/words/1", ""},
	}
	for _, at := range authTests {
		t.Run("unauthorized_"+at.method, func(t *testing.T) {
			var body io.Reader
			if at.body != "" {
				body = strings.NewReader(at.body)
			}
			req, _ := http.NewRequest(at.method, at.path, body)
			if at.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("%s: %v", at.method, err)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s without auth: got %d, want %d", at.method, resp.StatusCode, http.StatusUnauthorized)
			}
			resp.Body.Close()
		})
	}
}
