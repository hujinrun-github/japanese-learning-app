package user

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/httputil"
)

// UserHandler handles HTTP requests for the user module.
type UserHandler struct {
	svc *UserService
}

// NewUserHandler creates a UserHandler.
func NewUserHandler(svc *UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// RegisterRoutes registers user routes (public and protected).
// Public routes:
//
//	POST /api/v1/auth/register  → Register
//	POST /api/v1/auth/login     → Login
//
// Protected routes (require AuthMiddleware upstream):
//
//	GET  /api/v1/users/me       → GetProfile
func (h *UserHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/register", h.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/login", h.handleLogin)
	mux.HandleFunc("GET /api/v1/users/me", h.handleGetProfile)
}

// handleRegister handles POST /api/v1/auth/register
func (h *UserHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if req.Email == "" || req.Password == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "email and password are required", "")
		return
	}

	u, err := h.svc.Register(req)
	if err != nil {
		slog.Error("handleRegister failed", "err", err)
		httputil.WriteError(w, http.StatusConflict, "ERR_CONFLICT", "registration failed", "")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, httputil.APIResponse{Data: u})
}

// handleLogin handles POST /api/v1/auth/login
func (h *UserHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if req.Email == "" || req.Password == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "email and password are required", "")
		return
	}

	resp, err := h.svc.Login(req)
	if err != nil {
		slog.Error("handleLogin failed", "err", err, "email", req.Email)
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "invalid credentials", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: resp})
}

// handleGetProfile handles GET /api/v1/users/me
func (h *UserHandler) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	u, err := h.svc.GetProfile(userID)
	if err != nil {
		slog.Error("handleGetProfile failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "internal server error", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: u})
}
