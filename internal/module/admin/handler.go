package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/speaking"
)

// HandlerConfig holds all dependencies for admin handlers.
type HandlerConfig struct {
	AdminToken       string
	WordStore        *data.WordStore
	GrammarStore     *data.GrammarStore
	SpeakingStore    *data.SpeakingStore
	WritingStore     *data.WritingStore
	TranslationStore *data.TranslationStore
	UserStore        *data.UserStore
	DB               *sql.DB
	AIAPIKey         string
	AIAPIEndpoint    string
	AIModel          string
}

// Handler groups all admin HTTP handlers.
type Handler struct {
	cfg HandlerConfig
}

// NewHandler creates a Handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{cfg: cfg}
}

// RegisterRoutes sets up all admin routes under /api/admin/.
func (h *Handler) RegisterRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	// Words
	mux.HandleFunc("GET /api/admin/words", h.auth(h.listWords))
	mux.HandleFunc("POST /api/admin/words", h.auth(h.createWord))
	mux.HandleFunc("PUT /api/admin/words/{id}", h.auth(h.updateWord))
	mux.HandleFunc("DELETE /api/admin/words/{id}", h.auth(h.deleteWord))
	// Grammar
	mux.HandleFunc("GET /api/admin/grammar", h.auth(h.listGrammar))
	mux.HandleFunc("POST /api/admin/grammar", h.auth(h.createGrammar))
	mux.HandleFunc("PUT /api/admin/grammar/{id}", h.auth(h.updateGrammar))
	mux.HandleFunc("DELETE /api/admin/grammar/{id}", h.auth(h.deleteGrammar))
	// Speaking
	mux.HandleFunc("GET /api/admin/speaking", h.auth(h.listSpeaking))
	mux.HandleFunc("POST /api/admin/speaking", h.auth(h.createSpeaking))
	mux.HandleFunc("PUT /api/admin/speaking/{id}", h.auth(h.updateSpeaking))
	mux.HandleFunc("DELETE /api/admin/speaking/{id}", h.auth(h.deleteSpeaking))
	// Writing
	mux.HandleFunc("GET /api/admin/writing", h.auth(h.listWriting))
	mux.HandleFunc("POST /api/admin/writing", h.auth(h.createWriting))
	mux.HandleFunc("PUT /api/admin/writing/{id}", h.auth(h.updateWriting))
	mux.HandleFunc("DELETE /api/admin/writing/{id}", h.auth(h.deleteWriting))
	// Translation
	mux.HandleFunc("GET /api/admin/translation", h.auth(h.listTranslation))
	mux.HandleFunc("POST /api/admin/translation", h.auth(h.createTranslation))
	mux.HandleFunc("PUT /api/admin/translation/{id}", h.auth(h.updateTranslation))
	mux.HandleFunc("DELETE /api/admin/translation/{id}", h.auth(h.deleteTranslation))
	// Users
	mux.HandleFunc("GET /api/admin/users", h.auth(h.listUsers))
	mux.HandleFunc("GET /api/admin/users/{id}/stats", h.auth(h.getUserStats))
	// Records
	mux.HandleFunc("GET /api/admin/records/{module}", h.auth(h.listRecords))
	// Import
	mux.HandleFunc("POST /api/admin/import/{module}", h.auth(h.bulkImport))
	// Audio
	mux.HandleFunc("POST /api/admin/audio/regen", h.auth(h.regenerateAudio))
	// TTS defaults
	mux.HandleFunc("GET /api/admin/tts-defaults", h.auth(h.ttsDefaults))
	return mux
}

// auth wraps a handler with admin token verification.
func (h *Handler) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != h.cfg.AdminToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (h *Handler) ttsDefaults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"tts_url": speaking.DefaultVLLMTTSURL(),
	})
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

