package flowspace

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/testdb"
)

func TestHTTPPairingKeepsSecretsServerSideAndScopesUser(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'pair@example.test','test'),(2,'other@example.test','test')`); err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input["source_user_id"] != "1" || input["source_instance_id"] != "japanese-test" {
			t.Errorf("unscoped identity: %v", input)
		}
		io.WriteString(w, `{"data":{"connection_id":"conn-http","key_id":"v1","secret":"private-signing-secret","workspace_id":"workspace-test","workspace_name":"Test"}}`)
	}))
	defer remote.Close()
	svc, err := NewService(db, Options{BaseURL: remote.URL, SourceInstanceID: "japanese-test", MasterKey: []byte(strings.Repeat("k", 32))})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewHandler(svc).RegisterRoutes(mux)
	protected := user.AuthMiddleware("test-jwt", mux)
	request := func(method, path string, uid int64, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if uid > 0 {
			token, _, err := user.SignToken(uid, "test-jwt", time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		protected.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		method, path string
		uid          int64
		body         string
		want         int
	}{
		{"GET", "/api/v1/integrations/flowspace", 0, "", 401},
		{"POST", "/api/v1/integrations/flowspace/pair", 1, `{"pairing_code":"valid-code-at-least-16","source_user_id":"2"}`, 400},
		{"POST", "/api/v1/integrations/flowspace/pair", 1, `{"pairing_code":"valid-code-at-least-16"}`, 200},
		{"GET", "/api/v1/integrations/flowspace", 2, "", 200},
		{"POST", "/api/v1/integrations/flowspace/retry", 2, "", 200},
	} {
		w := request(tc.method, tc.path, tc.uid, tc.body)
		if w.Code != tc.want {
			t.Fatalf("%s %s status=%d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "private-signing-secret") || strings.Contains(w.Body.String(), "encrypted_secret") || strings.Contains(w.Body.String(), "master_key") {
			t.Fatal("credential leaked to browser")
		}
		if tc.uid == 2 && strings.Contains(w.Body.String(), "conn-http") {
			t.Fatal("connection leaked to another user")
		}
	}
	if _, err := svc.Status(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}
