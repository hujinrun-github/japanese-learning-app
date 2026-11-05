package writing

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

// WritingHandler handles HTTP requests for the writing module.
type WritingHandler struct {
	svc    *WritingService
	streak user.StreakRecorder
}

// NewWritingHandler creates a WritingHandler.
func NewWritingHandler(svc *WritingService) *WritingHandler {
	return &WritingHandler{svc: svc}
}

// SetStreakRecorder sets an optional recorder used to keep the study streak
// current after an answer is submitted. Nil disables check-ins.
func (h *WritingHandler) SetStreakRecorder(r user.StreakRecorder) {
	h.streak = r
}

// RegisterRoutes registers writing routes.
// Routes:
//
//	GET  /api/v1/writing/queue           → GetDailyQueue
//	POST /api/v1/writing/input           → SubmitInput
//	POST /api/v1/writing/sentence        → SubmitSentence
//	GET  /api/v1/writing/topics          → ListTopics
//	POST /api/v1/writing/topics/{id}     → SubmitTopic
//	GET  /api/v1/writing/records         → ListRecords
func (h *WritingHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/writing/queue", h.handleGetDailyQueue)
	mux.HandleFunc("POST /api/v1/writing/input", h.handleSubmitInput)
	mux.HandleFunc("POST /api/v1/writing/sentence", h.handleSubmitSentence)
	mux.HandleFunc("GET /api/v1/writing/topics", h.handleListTopics)
	mux.HandleFunc("POST /api/v1/writing/topics/{id}", h.handleSubmitTopic)
	mux.HandleFunc("GET /api/v1/writing/records", h.handleListRecords)
}

// handleGetDailyQueue handles GET /api/v1/writing/queue
func (h *WritingHandler) handleGetDailyQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	questions, err := h.svc.GetDailyQueue(userID)
	if err != nil {
		slog.Error("handleGetDailyQueue failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load writing queue", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: questions})
}

// submitInputRequest is the request body for POST /api/v1/writing/input.
type submitInputRequest struct {
	QuestionID int64  `json:"question_id"`
	Question   string `json:"question"`
	UserAnswer string `json:"user_answer"`
}

// handleSubmitInput handles POST /api/v1/writing/input
func (h *WritingHandler) handleSubmitInput(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var req submitInputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if req.QuestionID == 0 || req.Question == "" || req.UserAnswer == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "question_id, question and user_answer are required", "")
		return
	}

	rec, err := h.svc.SubmitInput(userID, req.QuestionID, req.Question, req.UserAnswer)
	if err != nil {
		slog.Error("handleSubmitInput failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to submit input answer", "")
		return
	}

	user.RecordStudyDayBestEffort(h.streak, userID)

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: rec})
}

// submitSentenceRequest is the request body for POST /api/v1/writing/sentence.
type submitSentenceRequest struct {
	Question   string `json:"question"`
	UserAnswer string `json:"user_answer"`
}

// handleSubmitSentence handles POST /api/v1/writing/sentence
func (h *WritingHandler) handleSubmitSentence(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var req submitSentenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if req.Question == "" || req.UserAnswer == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "question and user_answer are required", "")
		return
	}

	rec, err := h.svc.SubmitSentence(userID, req.Question, req.UserAnswer)
	if err != nil {
		slog.Error("handleSubmitSentence failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to submit sentence answer", "")
		return
	}

	user.RecordStudyDayBestEffort(h.streak, userID)

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: rec})
}

// handleListRecords handles GET /api/v1/writing/records
func (h *WritingHandler) handleListRecords(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	records, err := h.svc.ListRecords(userID)
	if err != nil {
		slog.Error("handleListRecords failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load writing records", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: records})
}

func (h *WritingHandler) handleListTopics(w http.ResponseWriter, r *http.Request) {
	if _, ok := user.UserIDFromContext(r.Context()); !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: writingTopics()})
}

func (h *WritingHandler) handleSubmitTopic(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	var req struct {
		Type       WritingType `json:"type"`
		UserAnswer string      `json:"user_answer"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("handleSubmitTopic: invalid request", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	answer := strings.TrimSpace(req.UserAnswer)
	if (req.Type != WritingTypeEssay && req.Type != WritingTypeSentence) || answer == "" || utf8.RuneCountInString(answer) > 1000 {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "type must be essay or sentence and user_answer must be 1–1000 characters", "")
		return
	}

	topicID := r.PathValue("id")
	for _, topic := range writingTopics() {
		if topic.ID != topicID {
			continue
		}
		rec, err := h.svc.submitTopic(userID, topic, req.Type, answer)
		if err != nil {
			slog.Error("handleSubmitTopic failed", "err", err, "user_id", userID, "topic_id", topicID)
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to submit writing", "")
			return
		}
		user.RecordStudyDayBestEffort(h.streak, userID)
		httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: rec})
		return
	}
	httputil.WriteError(w, http.StatusNotFound, "ERR_NOT_FOUND", "writing topic not found", "")
}
