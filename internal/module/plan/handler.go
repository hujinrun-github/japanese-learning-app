package plan

import (
	"log/slog"
	"net/http"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

// PlanHandler exposes the daily plan over HTTP.
type PlanHandler struct {
	svc *PlanService
}

// NewPlanHandler creates a PlanHandler.
func NewPlanHandler(svc *PlanService) *PlanHandler {
	return &PlanHandler{svc: svc}
}

// RegisterRoutes registers plan routes.
// Routes:
//
//	GET /api/v1/daily-plan → today's cross-module study plan
func (h *PlanHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/daily-plan", h.handleGetDailyPlan)
}

// handleGetDailyPlan handles GET /api/v1/daily-plan
func (h *PlanHandler) handleGetDailyPlan(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	plan, err := h.svc.GetDailyPlan(userID)
	if err != nil {
		slog.Error("handleGetDailyPlan failed", "err", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to build daily plan", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: plan})
}
