package user

import (
	"encoding/json"
	"errors"
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
//	POST /api/v1/auth/register          → Register
//	POST /api/v1/auth/login             → Login
//	POST /api/v1/auth/forgot-password   → ForgotPassword
//	POST /api/v1/auth/reset-password    → ResetPassword
//
// Protected routes (require AuthMiddleware upstream):
//
//	GET  /api/v1/users/me               → GetProfile
func (h *UserHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/register", h.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/login", h.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/forgot-password", h.handleForgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset-password", h.handleResetPassword)
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
		if errors.Is(err, ErrEmailTaken) {
			httputil.WriteError(w, http.StatusConflict, "ERR_EMAIL_TAKEN", "email already registered", "")
		} else {
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to create account", "")
		}
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

// handleForgotPassword handles POST /api/v1/auth/forgot-password
func (h *UserHandler) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if req.Email == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "email is required", "")
		return
	}

	slog.Debug("handleForgotPassword called", "email", req.Email)
	if err := h.svc.ForgotPassword(req.Email); err != nil {
		slog.Error("handleForgotPassword failed", "err", err, "email", req.Email)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to process request", "")
		return
	}

	// Always return 200 to avoid email enumeration.
	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{
		"message": "If that email is registered, a reset link has been sent.",
	}})
}

// handleResetPassword handles POST /api/v1/auth/reset-password
func (h *UserHandler) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "invalid request body", "")
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		httputil.WriteError(w, http.StatusBadRequest, "ERR_BAD_REQUEST", "token and new_password are required", "")
		return
	}

	slog.Debug("handleResetPassword called")
	if err := h.svc.ResetPassword(req.Token, req.NewPassword); err != nil {
		slog.Error("handleResetPassword failed", "err", err)
		if errors.Is(err, ErrTokenInvalid) {
			httputil.WriteError(w, http.StatusBadRequest, "ERR_TOKEN_INVALID", "invalid or expired reset token", "")
		} else {
			httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to reset password", "")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: map[string]string{
		"message": "Password reset successfully.",
	}})
}
