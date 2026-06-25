package shadowing

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/user"
)

type fakeShadowingService struct {
	session     *ShadowingSession
	progress    *Progress
	sessionErr  error
	progressErr error
	attemptErr  error

	gotUserID   int64
	gotLessonID int64
	gotProgress ProgressRequest
	gotAttempt  AttemptRequest
}

func (f *fakeShadowingService) GetSession(userID, lessonID int64) (*ShadowingSession, error) {
	f.gotUserID = userID
	f.gotLessonID = lessonID
	if f.sessionErr != nil {
		return nil, f.sessionErr
	}
	return f.session, nil
}

func (f *fakeShadowingService) SaveProgress(userID, lessonID int64, req ProgressRequest) (*Progress, error) {
	f.gotUserID = userID
	f.gotLessonID = lessonID
	f.gotProgress = req
	if f.progressErr != nil {
		return nil, f.progressErr
	}
	return f.progress, nil
}

func (f *fakeShadowingService) SaveAttempt(userID, lessonID int64, req AttemptRequest) error {
	f.gotUserID = userID
	f.gotLessonID = lessonID
	f.gotAttempt = req
	return f.attemptErr
}

func TestHandler_MissingContextUserReturnsUnauthorized(t *testing.T) {
	mux := http.NewServeMux()
	NewHandler(&fakeShadowingService{}).RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lessons/1/shadowing", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	var body errorResponse
	decodeJSON(t, rec, &body)
	if body.Code != ErrUnauthorized {
		t.Fatalf("code = %s, want %s", body.Code, ErrUnauthorized)
	}
}

func TestHandler_GetSessionReturnsSessionJSON(t *testing.T) {
	fake := &fakeShadowingService{
		session: &ShadowingSession{
			Lesson: &lesson.Lesson{
				LessonSummary: lesson.LessonSummary{ID: 1, Title: "Shadowing Lesson"},
			},
			Progress:                 nil,
			AttemptSummary:           []SentenceAttemptSummary{},
			CompletedSentenceIndexes: []int{},
		},
	}
	mux := authenticatedShadowingMux(t, fake)

	rec := httptest.NewRecorder()
	req := authenticatedRequest(t, http.MethodGet, "/api/v1/lessons/1/shadowing", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fake.gotUserID != 42 || fake.gotLessonID != 1 {
		t.Fatalf("service called with user=%d lesson=%d, want user=42 lesson=1", fake.gotUserID, fake.gotLessonID)
	}

	var body apiResponse
	decodeJSON(t, rec, &body)
	data := body.Data.(map[string]any)
	if _, ok := data["progress"]; !ok {
		t.Fatalf("session data = %+v, missing progress field", data)
	}
	lessonData := data["lesson"].(map[string]any)
	if lessonData["id"].(float64) != 1 {
		t.Fatalf("lesson id = %v, want 1", lessonData["id"])
	}
}

func TestHandler_SaveProgressStaleVersionReturnsDetails(t *testing.T) {
	fake := &fakeShadowingService{
		progressErr: &Error{
			Code:                    ErrShadowingVersionStale,
			Message:                 "shadowing version is stale",
			CurrentShadowingVersion: 5,
		},
	}
	mux := authenticatedShadowingMux(t, fake)
	body := []byte(`{"shadowing_version":4,"last_sentence_index":1,"last_position_ms":1200,"last_practice_mode":"loop"}`)

	rec := httptest.NewRecorder()
	req := authenticatedRequest(t, http.MethodPost, "/api/v1/lessons/1/shadowing/progress", bytes.NewReader(body))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if fake.gotProgress.Version != 4 || fake.gotProgress.PracticeMode != PracticeModeLoop {
		t.Fatalf("progress request = %+v, want decoded stale request", fake.gotProgress)
	}

	var response errorResponse
	decodeJSON(t, rec, &response)
	if response.Code != ErrShadowingVersionStale {
		t.Fatalf("code = %s, want %s", response.Code, ErrShadowingVersionStale)
	}
	if response.Details["current_shadowing_version"].(float64) != 5 {
		t.Fatalf("details = %+v, want current_shadowing_version=5", response.Details)
	}
}

func TestHandler_SaveAttemptNormalModeReturnsInvalidMode(t *testing.T) {
	fake := &fakeShadowingService{
		attemptErr: &Error{
			Code:    ErrShadowingAttemptModeInvalid,
			Message: "shadowing attempt mode must be loop or record",
		},
	}
	mux := authenticatedShadowingMux(t, fake)
	body := []byte(`{"shadowing_version":5,"sentence_index":0,"practice_mode":"normal","playback_rate":1,"loop_count":0}`)

	rec := httptest.NewRecorder()
	req := authenticatedRequest(t, http.MethodPost, "/api/v1/lessons/1/shadowing/attempts", bytes.NewReader(body))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	if fake.gotAttempt.PracticeMode != PracticeModeNormal {
		t.Fatalf("attempt request = %+v, want normal mode decoded", fake.gotAttempt)
	}

	var response errorResponse
	decodeJSON(t, rec, &response)
	if response.Code != ErrShadowingAttemptModeInvalid {
		t.Fatalf("code = %s, want %s", response.Code, ErrShadowingAttemptModeInvalid)
	}
}

type apiResponse struct {
	Data any `json:"data"`
}

type errorResponse struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func authenticatedShadowingMux(t *testing.T, svc ServiceInterface) *http.ServeMux {
	t.Helper()
	protected := http.NewServeMux()
	NewHandler(svc).RegisterRoutes(protected)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/lessons/", user.AuthMiddleware("test-secret", protected))
	return mux
}

func authenticatedRequest(t *testing.T, method, target string, body *bytes.Reader) *http.Request {
	t.Helper()
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		requestBody = body
	}
	req := httptest.NewRequest(method, target, requestBody)
	token, _, err := user.SignToken(42, "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("SignToken error: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("json.Unmarshal(%s) error: %v", rec.Body.String(), err)
	}
}
