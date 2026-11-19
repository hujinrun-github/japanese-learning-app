package flowspace

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/studyplan"
	"japanese-learning-app/internal/testdb"
)

func TestEncryptedPairAndReliableDelivery(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'flowspace@example.test','test'),(2,'other@example.test','test')`); err != nil {
		t.Fatal(err)
	}
	p := studyplan.Plan{Title: "Study", StartDate: "2026-11-19", EndDate: "2026-11-30", Timezone: "Asia/Shanghai", Weekdays: []int{1, 2, 3, 4, 5}, TimeSlots: []studyplan.TimeSlot{{ID: "evening", Title: "Evening", Time: "18:40", Duration: 20, Tasks: []studyplan.Task{{ID: "listening", Title: "Listening", Path: "/listening", Minutes: 20}}}}}
	raw, _ := json.Marshal(p)
	if _, err := db.Exec(`INSERT INTO study_plans(user_id,plan_json,calendar_token) VALUES(1,$1,'worker-test')`, string(raw)); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 19, 11, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	var nonces, eventIDs []string
	var events int
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/integrations/japanese/pair" {
			io.WriteString(w, `{"data":{"connection_id":"conn-test","key_id":"v1","secret":"test-signing-secret","workspace_id":"workspace-test","workspace_name":"Test"}}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		sum := sha256.Sum256(body)
		canonical := strings.Join([]string{r.Method, r.URL.RequestURI(), "conn-test", "v1", r.Header.Get("X-FlowSpace-Timestamp"), r.Header.Get("X-FlowSpace-Nonce"), hex.EncodeToString(sum[:])}, "\n")
		mac := hmac.New(sha256.New, []byte("test-signing-secret"))
		mac.Write([]byte(canonical))
		if r.Header.Get("X-FlowSpace-Signature") != hex.EncodeToString(mac.Sum(nil)) {
			t.Error("invalid signing contract")
			w.WriteHeader(401)
			return
		}
		if r.Method == "GET" {
			io.WriteString(w, `{"data":{"status":"applied"}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/catalog") {
			w.WriteHeader(200)
			io.WriteString(w, `{"data":{"status":"applied"}}`)
			return
		}
		var e studyplan.CompletionEvent
		json.Unmarshal(body, &e)
		mu.Lock()
		defer mu.Unlock()
		events++
		eventIDs = append(eventIDs, e.EventID)
		nonces = append(nonces, r.Header.Get("X-FlowSpace-Nonce"))
		if events == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(202)
		io.WriteString(w, `{"data":{"status":"received"}}`)
	}))
	defer remote.Close()
	svc, err := NewService(db, Options{BaseURL: remote.URL, SourceInstanceID: "japanese-test", MasterKey: []byte(strings.Repeat("k", 32)), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	status, err := svc.Pair(context.Background(), 1, "one-time-code-at-least-16")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Connected || status.WorkspaceID != "workspace-test" {
		t.Fatalf("status=%+v", status)
	}
	var encrypted string
	if err := db.QueryRow(`SELECT encrypted_secret FROM flowspace_connections WHERE user_id=1`).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encrypted, "test-signing-secret") {
		t.Fatal("secret not encrypted")
	}
	if other, err := svc.Status(context.Background(), 2); err != nil || other.Connected {
		t.Fatalf("cross-user status=%+v %v", other, err)
	}
	if worked, err := svc.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("catalog worker: %v %v", worked, err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := studyplan.SetTaskCompletion(context.Background(), tx, 1, "2026-11-19", "evening", "listening", true, "manual", now); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if worked, err := svc.RunOnce(context.Background()); err == nil || !worked {
		t.Fatalf("first delivery should persist retry: %v %v", worked, err)
	}
	now = now.Add(time.Hour)
	if worked, err := svc.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("retry worker: %v %v", worked, err)
	}
	if len(eventIDs) != 2 || eventIDs[0] != eventIDs[1] || nonces[0] == nonces[1] {
		t.Fatalf("retry identity/nonce=%v %v", eventIDs, nonces)
	}
	now = now.Add(time.Minute)
	if _, err := svc.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err = svc.Status(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if status.PendingCount != 0 || status.DeliveredCount != 2 || len(status.Events) == 0 || status.Events[0].RemoteStatus != "applied" {
		t.Fatalf("status=%+v", status)
	}
	if err := svc.Disconnect(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	status, err = svc.Status(context.Background(), 1)
	if err != nil || status.Connected {
		t.Fatalf("disconnect=%+v %v", status, err)
	}
}

func TestOptionsValidateEndpointAndMasterKey(t *testing.T) {
	for _, tc := range []struct {
		base, instance string
		key            []byte
		wantErr        bool
	}{
		{"", "", nil, false},
		{"http://192.168.1.13:4101", "japanese-test", make([]byte, 32), true},
		{"https://example.test", "", make([]byte, 32), true},
		{"https://example.test", "japanese-test", nil, true},
		{"http://127.0.0.1:4101", "japanese-test", make([]byte, 32), false},
		{"https://example.test/all-note", "japanese-test", make([]byte, 32), false},
	} {
		_, err := NewService(nil, Options{BaseURL: tc.base, SourceInstanceID: tc.instance, MasterKey: tc.key})
		if (err != nil) != tc.wantErr {
			t.Errorf("base %q: %v", tc.base, err)
		}
	}
}

func TestPairingDoesNotFollowRedirect(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'redirect@example.test','test')`); err != nil {
		t.Fatal(err)
	}
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true; w.WriteHeader(200) }))
	defer other.Close()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer remote.Close()
	svc, err := NewService(db, Options{BaseURL: remote.URL, SourceInstanceID: "japanese-test", MasterKey: []byte(strings.Repeat("k", 32)), HTTPClient: remote.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Pair(context.Background(), 1, "private-pairing-code-at-least-16"); err == nil {
		t.Fatal("redirect accepted")
	}
	if leaked {
		t.Fatal("pairing credential followed a redirect")
	}
}

func TestNeedsAttentionReceiptCanRecoverWithoutReposting(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		name := "background"
		if explicit {
			name = "explicit retry"
		}
		t.Run(name, func(t *testing.T) {
			db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
			if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'receipt-recovery@example.test','test')`); err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 11, 19, 11, 0, 0, 0, time.UTC)
			posts := 0
			remoteState := "needs_attention"
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts++
					w.WriteHeader(http.StatusAccepted)
					io.WriteString(w, `{"data":{"status":"received"}}`)
					return
				}
				if remoteState == "needs_attention" {
					io.WriteString(w, `{"data":{"status":"needs_attention","error_code":"completion_requirements"}}`)
					return
				}
				io.WriteString(w, `{"data":{"status":"applied"}}`)
			}))
			defer remote.Close()
			svc, err := NewService(db, Options{BaseURL: remote.URL, SourceInstanceID: "japanese-test", MasterKey: []byte(strings.Repeat("k", 32)), Now: func() time.Time { return now }})
			if err != nil {
				t.Fatal(err)
			}
			encrypted, err := svc.encrypt(1, "conn-recovery", "signing-secret")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO flowspace_connections(user_id,connection_id,key_id,source_instance_id,workspace_id,encrypted_secret) VALUES(1,'conn-recovery','v1','japanese-test','workspace-test',$1)`, encrypted); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO flowspace_outbox(event_id,user_id,connection_id,kind,payload_json,next_attempt_at) VALUES('recovery-event',1,'conn-recovery','event','{}',$1)`, now); err != nil {
				t.Fatal(err)
			}
			if worked, err := svc.RunOnce(context.Background()); err != nil || !worked {
				t.Fatalf("delivery=%v %v", worked, err)
			}
			now = now.Add(10 * time.Second)
			if worked, err := svc.RunOnce(context.Background()); err != nil || !worked {
				t.Fatalf("attention receipt=%v %v", worked, err)
			}
			before, err := svc.Status(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if before.Events[0].RemoteStatus != "needs_attention" {
				t.Fatalf("before=%+v", before)
			}
			remoteState = "applied"
			if explicit {
				if err := svc.Retry(context.Background(), 1); err != nil {
					t.Fatal(err)
				}
			} else {
				now = now.Add(61 * time.Second)
			}
			if worked, err := svc.RunOnce(context.Background()); err != nil || !worked {
				t.Fatalf("recovery receipt=%v %v", worked, err)
			}
			after, err := svc.Status(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if after.Events[0].RemoteStatus != "applied" || after.Events[0].Error != "" || posts != 1 {
				t.Fatalf("after=%+v posts=%d", after, posts)
			}
		})
	}
}
