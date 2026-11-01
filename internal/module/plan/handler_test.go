package plan

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
)

const testJWTSecret = "test-secret"

// authenticatedPlanMux wires the handler behind the real auth middleware so
// the test exercises the same path production requests take.
func authenticatedPlanMux(svc *PlanService) *http.ServeMux {
	protected := http.NewServeMux()
	NewPlanHandler(svc).RegisterRoutes(protected)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/daily-plan", user.AuthMiddleware(testJWTSecret, protected))
	return mux
}

func authenticatedPlanRequest(t *testing.T) *http.Request {
	t.Helper()
	token, _, err := user.SignToken(42, testJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("SignToken error: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/daily-plan", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// TestHandlerRequiresAuth guards the route against anonymous access.
func TestHandlerRequiresAuth(t *testing.T) {
	svc, _ := newTestService(statsWith(map[string]user.ModuleStat{
		"word": {DailyGoal: 20},
	}), nil)

	rec := httptest.NewRecorder()
	authenticatedPlanMux(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/daily-plan", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandlerReturnsPlan pins the response envelope and item shape.
func TestHandlerReturnsPlan(t *testing.T) {
	svc, _ := newTestService(statsWith(map[string]user.ModuleStat{
		"word":    {DailyGoal: 20, TodayCompleted: 4, DueCount: 12},
		"writing": {DailyGoal: 3, TodayCompleted: 0},
	}), nil)

	rec := httptest.NewRecorder()
	authenticatedPlanMux(svc).ServeHTTP(rec, authenticatedPlanRequest(t))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var envelope struct {
		Data DailyPlan `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
	}

	got := envelope.Data
	if got.Date == "" || got.Timezone == "" {
		t.Errorf("Date/Timezone not populated: %+v", got)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items length = %d, want 2", len(got.Items))
	}
	// word is already started (4/20) so it leads; writing (0/3) follows.
	if got.Items[0].Key != ModuleWord {
		t.Errorf("Items[0].Key = %q, want word (started modules lead)", got.Items[0].Key)
	}
	if got.Items[0].Path != "/words/review" {
		t.Errorf("Items[0].Path = %q, want /words/review", got.Items[0].Path)
	}
	if got.Items[0].Status != StatusInProgress || got.Items[0].Remaining != 16 {
		t.Errorf("Items[0] = %+v, want in_progress with 16 remaining", got.Items[0])
	}
	if got.Items[1].Status != StatusTodo || got.Items[1].Remaining != 3 {
		t.Errorf("Items[1] = %+v, want todo with 3 remaining", got.Items[1])
	}
}

// TestHandlerReportsStoreFailure keeps storage errors from leaking as 200s.
func TestHandlerReportsStoreFailure(t *testing.T) {
	svc, _ := newTestService(nil, errTestStoreDown)

	rec := httptest.NewRecorder()
	authenticatedPlanMux(svc).ServeHTTP(rec, authenticatedPlanRequest(t))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// TestNewPlanServiceFallsBackToLocalTimezone keeps a nil location safe.
func TestNewPlanServiceFallsBackToLocalTimezone(t *testing.T) {
	svc := NewPlanService(&fakeStatsProvider{stats: statsWith(nil)}, nil)
	if svc.loc == nil {
		t.Fatal("loc = nil, want time.Local fallback")
	}
	if got := resolvePlanDay(fixedNow(), svc.loc); got.date == "" {
		t.Error("resolvePlanDay returned an empty date")
	}
}
