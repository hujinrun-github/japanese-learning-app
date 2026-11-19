package studyplan

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

// TaskDB lets a completed training update the study task in the same database
// transaction as its attempt. Both *sql.DB and *sql.Tx satisfy this interface.
type TaskDB interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// CompleteListeningTask marks the built-in daily listening task, if scheduled
// today in the saved plan's timezone. It never creates or rewrites a plan.
func CompleteListeningTask(ctx context.Context, db TaskDB, uid int64, now time.Time) error {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT plan_json FROM study_plans WHERE user_id=$1`, uid).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("read listening study plan: %w", err)
	}
	var p Plan
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return fmt.Errorf("decode listening study plan: %w", err)
	}
	today, err := p.Today(now)
	if err != nil {
		return fmt.Errorf("resolve listening study day: %w", err)
	}
	for _, slot := range today.Slots {
		for _, task := range slot.Tasks {
			if task.ID != "listening" || !validTaskPath(task.Path) || !(task.Path == "/listening" || strings.HasPrefix(task.Path, "/listening?") || strings.HasPrefix(task.Path, "/listening/")) {
				continue
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO study_plan_task_completions(user_id,study_date,slot_id,task_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, uid, today.Date, slot.ID, task.ID); err != nil {
				return fmt.Errorf("complete listening study task: %w", err)
			}
			slog.Debug("listening study task completed", "user_id", uid, "study_date", today.Date, "slot_id", slot.ID)
			return nil
		}
	}
	return nil
}

func (h *Handler) readToday(ctx context.Context, uid int64) (TodayPlan, error) {
	var raw string
	err := h.db.QueryRowContext(ctx, `SELECT plan_json FROM study_plans WHERE user_id=$1`, uid).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		now := h.now()
		return TodayPlan{Date: now.Format("2006-01-02"), Timezone: now.Location().String(), Reason: "no_plan", Slots: []TodaySlot{}}, nil
	}
	if err != nil {
		return TodayPlan{}, fmt.Errorf("read today's study plan: %w", err)
	}
	var p Plan
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return TodayPlan{}, fmt.Errorf("decode today's study plan: %w", err)
	}
	today, err := p.Today(h.now())
	if err != nil {
		return TodayPlan{}, err
	}
	if len(today.Slots) == 0 {
		return today, nil
	}
	rows, err := h.db.QueryContext(ctx, `SELECT slot_id,task_id FROM study_plan_task_completions WHERE user_id=$1 AND study_date=$2`, uid, today.Date)
	if err != nil {
		return TodayPlan{}, fmt.Errorf("read study task completions: %w", err)
	}
	defer rows.Close()
	completed := make(map[string]map[string]bool)
	for rows.Next() {
		var slotID, taskID string
		if err := rows.Scan(&slotID, &taskID); err != nil {
			return TodayPlan{}, fmt.Errorf("scan study task completion: %w", err)
		}
		if completed[slotID] == nil {
			completed[slotID] = make(map[string]bool)
		}
		completed[slotID][taskID] = true
	}
	if err := rows.Err(); err != nil {
		return TodayPlan{}, fmt.Errorf("iterate study task completions: %w", err)
	}
	for i := range today.Slots {
		for j := range today.Slots[i].Tasks {
			today.Slots[i].Tasks[j].Completed = completed[today.Slots[i].ID][today.Slots[i].Tasks[j].ID]
		}
	}
	return today, nil
}

func (h *Handler) today(w http.ResponseWriter, r *http.Request) {
	uid, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, 401, "ERR_UNAUTHORIZED", "请先登录", "")
		return
	}
	result, err := h.readToday(r.Context(), uid)
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httputil.WriteJSON(w, 200, httputil.APIResponse{Data: result})
}

func (h *Handler) completeTask(w http.ResponseWriter, r *http.Request) {
	uid, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, 401, "ERR_UNAUTHORIZED", "请先登录", "")
		return
	}
	var input struct {
		Date      string `json:"date"`
		Completed *bool  `json:"completed"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httputil.WriteError(w, 400, "ERR_BAD_REQUEST", "任务状态格式无效", "")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) || input.Completed == nil {
		httputil.WriteError(w, 400, "ERR_BAD_REQUEST", "任务状态格式无效", "")
		return
	}
	today, err := h.readToday(r.Context(), uid)
	if err != nil {
		h.fail(w, err)
		return
	}
	if input.Date != today.Date {
		httputil.WriteError(w, 409, "ERR_STUDY_DATE_CHANGED", "学习日期已变化，请刷新当天任务", "")
		return
	}
	slotID, taskID := r.PathValue("slotID"), r.PathValue("taskID")
	found := false
	for _, slot := range today.Slots {
		if slot.ID != slotID {
			continue
		}
		for _, task := range slot.Tasks {
			found = found || task.ID == taskID
		}
	}
	if !found {
		httputil.WriteError(w, 404, "ERR_STUDY_TASK_NOT_FOUND", "当天没有这项学习任务", "")
		return
	}
	if *input.Completed {
		_, err = h.db.ExecContext(r.Context(), `INSERT INTO study_plan_task_completions(user_id,study_date,slot_id,task_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, uid, today.Date, slotID, taskID)
	} else {
		_, err = h.db.ExecContext(r.Context(), `DELETE FROM study_plan_task_completions WHERE user_id=$1 AND study_date=$2 AND slot_id=$3 AND task_id=$4`, uid, today.Date, slotID, taskID)
	}
	if err != nil {
		h.fail(w, fmt.Errorf("save study task completion: %w", err))
		return
	}
	slog.Debug("study task completion saved", "user_id", uid, "study_date", today.Date, "slot_id", slotID, "task_id", taskID, "completed", *input.Completed)
	h.today(w, r)
}
