package flowspace

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/studyplan"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/testdb"
)

// TestFlowSpaceCrossProjectSource is launched by FlowSpace's retained
// TestJapaneseCrossProjectHTTP test. Both HTTP servers and PostgreSQL schemas
// are disposable; no application service, stored account, or live API is used.
func TestFlowSpaceCrossProjectSource(t *testing.T) {
	base := os.Getenv("FLOWSPACE_CROSS_PROJECT_URL")
	if base == "" {
		t.Skip("launched by all_note TestJapaneseCrossProjectHTTP with an isolated receiver")
	}
	dir := os.Getenv("FLOWSPACE_CROSS_PROJECT_HANDSHAKE")
	if dir == "" {
		t.Fatal("missing cross-project handshake directory")
	}
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(123,'cross-project@example.test','test')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	opts := Options{BaseURL: base, SourceInstanceID: "japanese-cross-project", MasterKey: bytes.Repeat([]byte{31}, 32), Now: func() time.Time { return now }}
	service, err := NewService(db, opts)
	if err != nil {
		t.Fatal(err)
	}
	protected, public := http.NewServeMux(), http.NewServeMux()
	studyplan.NewHandler(db).RegisterRoutes(protected, public)
	NewHandler(service).RegisterRoutes(protected)
	const jwtSecret = "cross-project-fixture-session"
	public.Handle("/api/v1/", user.AuthMiddleware(jwtSecret, protected))
	source := httptest.NewServer(public)
	defer source.Close()
	token, _, err := user.SignToken(123, jwtSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, input, output any) {
		t.Helper()
		var body bytes.Buffer
		if input != nil {
			if err := json.NewEncoder(&body).Encode(input); err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequestWithContext(t.Context(), method, source.URL+path, &body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		response, err := source.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("source %s %s: HTTP %d", method, path, response.StatusCode)
		}
		if output != nil {
			if err := json.NewDecoder(response.Body).Decode(output); err != nil {
				t.Fatal(err)
			}
		}
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	today := now.In(location).Format("2006-01-02")
	plan := studyplan.Plan{Title: "Cross-project daily listening", StartDate: today, EndDate: now.In(location).AddDate(0, 0, 2).Format("2006-01-02"), Timezone: "Asia/Shanghai", Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, TimeSlots: []studyplan.TimeSlot{{ID: "evening", Title: "Listening", Time: "19:00", Duration: 20, Tasks: []studyplan.Task{{ID: "listening", Title: "Listening practice", Path: "/listening", Minutes: 20}}}}}
	request(http.MethodPut, "/api/v1/study-plan", plan, nil)
	var pairing struct {
		Data Status `json:"data"`
	}
	request(http.MethodPost, "/api/v1/integrations/flowspace/pair", map[string]string{"pairing_code": os.Getenv("FLOWSPACE_CROSS_PROJECT_CODE")}, &pairing)
	if !pairing.Data.Connected {
		t.Fatal("pairing did not connect the source account")
	}
	if worked, err := service.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("catalog delivery worked=%v error=%v", worked, err)
	}
	var planID string
	if err := db.QueryRow(`SELECT plan_id FROM study_plans WHERE user_id=123`).Scan(&planID); err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path+".tmp", raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			t.Fatal(err)
		}
	}
	write("source.json", map[string]string{"connection_id": pairing.Data.ConnectionID, "plan_id": planID, "study_date": today})
	waitContext, stop := context.WithTimeout(t.Context(), 30*time.Second)
	defer stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(filepath.Join(dir, "binding-ready")); err == nil {
			break
		}
		select {
		case <-waitContext.Done():
			t.Fatal("receiver did not bind source task")
		case <-ticker.C:
		}
	}
	request(http.MethodPut, "/api/v1/study-plan/tasks/evening/listening", map[string]any{"date": today, "completed": true}, nil)
	// Duplicate successful user requests must not create another source event.
	request(http.MethodPut, "/api/v1/study-plan/tasks/evening/listening", map[string]any{"date": today, "completed": true}, nil)
	var eventID string
	var eventCount int
	if err := db.QueryRow(`SELECT count(*) FROM flowspace_outbox WHERE kind='event'`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("duplicate source completion created %d events", eventCount)
	}
	if err := db.QueryRow(`SELECT event_id FROM flowspace_outbox WHERE kind='event'`).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	now = time.Now()
	// Receiver accepts durably but intentionally drops this first HTTP response.
	if worked, err := service.RunOnce(t.Context()); !worked || err == nil {
		t.Fatalf("expected lost acknowledgement, worked=%v error=%v", worked, err)
	}
	// Recreate the real worker from durable source state and retry with a fresh
	// signature/nonce; receiver must keep one inbox event and one domain effect.
	now = now.Add(10 * time.Second)
	service, err = NewService(db, opts)
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := service.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("recovered delivery worked=%v error=%v", worked, err)
	}
	deadline := time.Now().Add(20 * time.Second)
	var remote string
	for time.Now().Before(deadline) {
		now = now.Add(6 * time.Second)
		if _, err := service.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT remote_status FROM flowspace_outbox WHERE event_id=$1`, eventID).Scan(&remote); err != nil {
			t.Fatal(err)
		}
		if remote == "applied" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if remote != "applied" {
		t.Fatalf("source never observed applied receipt: %s", remote)
	}
	var attempts int
	if err := db.QueryRow(`SELECT attempts FROM flowspace_outbox WHERE event_id=$1`, eventID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("delivery attempts=%d want 2", attempts)
	}
	write("result.json", map[string]any{"event_id": eventID, "connection_id": pairing.Data.ConnectionID, "study_date": today, "remote_status": remote, "attempts": attempts, "source_event_count": eventCount})
}
