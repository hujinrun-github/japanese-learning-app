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
	if pool, ok := db.(*sql.DB); ok {
		tx, err := pool.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin listening study task: %w", err)
		}
		defer tx.Rollback()
		if err := completeListeningTask(ctx, tx, uid, now); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit listening study task: %w", err)
		}
		return nil
	}
	return completeListeningTask(ctx, db, uid, now)
}

func completeListeningTask(ctx context.Context, db TaskDB, uid int64, now time.Time) error {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT plan_json FROM study_plans WHERE user_id=$1 FOR UPDATE`, uid).Scan(&raw); err != nil {
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
			if err := SetTaskCompletion(ctx, db, uid, today.Date, slot.ID, task.ID, true, "listening_training", now); err != nil {
				return fmt.Errorf("complete listening study task: %w", err)
			}
			slog.Debug("listening study task completed", "user_id", uid, "study_date", today.Date, "slot_id", slot.ID)
			return nil
		}
	}
	return nil
}

type todayTaskDB interface {
	TaskDB
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (h *Handler) readToday(ctx context.Context, uid int64) (TodayPlan, error) {
	now := h.now()
	today, err := h.readTodayState(ctx, h.db, uid, now, false)
	if err != nil {
		return TodayPlan{}, err
	}
	// The same idempotent command also runs just after practice is recorded.
	for _, slot := range today.Slots {
		for _, task := range slot.Tasks {
			if _, _, changed := task.practiceCompletionChange(); changed {
				return h.updatePracticeCheckIns(ctx, uid, now)
			}
		}
	}
	return today, nil
}

func (h *Handler) readTodayState(ctx context.Context, db todayTaskDB, uid int64, now time.Time, lock bool) (TodayPlan, error) {
	query := `SELECT plan_json,plan_id FROM study_plans WHERE user_id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var raw, planID string
	err := db.QueryRowContext(ctx, query, uid).Scan(&raw, &planID)
	if errors.Is(err, sql.ErrNoRows) {
		return TodayPlan{Date: now.Format("2006-01-02"), Timezone: now.Location().String(), Reason: "no_plan", Slots: []TodaySlot{}}, nil
	}
	if err != nil {
		return TodayPlan{}, fmt.Errorf("read today's study plan: %w", err)
	}
	var p Plan
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return TodayPlan{}, fmt.Errorf("decode today's study plan: %w", err)
	}
	today, err := p.Today(now)
	if err != nil {
		return TodayPlan{}, err
	}
	if len(today.Slots) != 0 {
		completions, err := readCompletionReasons(ctx, db, uid, planID, today.Date)
		if err != nil {
			return TodayPlan{}, err
		}
		for i := range today.Slots {
			for j := range today.Slots[i].Tasks {
				task := &today.Slots[i].Tasks[j]
				task.CompletionReason, task.Completed = completions[[2]string{today.Slots[i].ID, task.ID}]
			}
		}
	}
	if err := attachPracticeProgress(ctx, db, uid, now, h.wordTimezone, p, &today); err != nil {
		return TodayPlan{}, err
	}
	return today, nil
}

func readCompletionReasons(ctx context.Context, db todayTaskDB, uid int64, planID, date string) (map[[2]string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT c.slot_id,c.task_id,COALESCE(s.reason,'manual')
		FROM study_plan_task_completions c LEFT JOIN study_plan_task_states s
		  ON s.user_id=c.user_id AND s.plan_id=$3 AND s.study_date=c.study_date
		  AND s.slot_id=c.slot_id AND s.task_id=c.task_id AND s.completed
		WHERE c.user_id=$1 AND c.study_date=$2`, uid, date, planID)
	if err != nil {
		return nil, fmt.Errorf("read study task completions: %w", err)
	}
	defer rows.Close()
	completed := make(map[[2]string]string)
	for rows.Next() {
		var slotID, taskID, reason string
		if err := rows.Scan(&slotID, &taskID, &reason); err != nil {
			return nil, fmt.Errorf("scan study task completion: %w", err)
		}
		completed[[2]string{slotID, taskID}] = reason
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate study task completions: %w", err)
	}
	return completed, nil
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
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		h.fail(w, fmt.Errorf("begin study task completion: %w", err))
		return
	}
	defer tx.Rollback()
	err = SetTaskCompletion(r.Context(), tx, uid, today.Date, slotID, taskID, *input.Completed, "manual", h.now())
	if errors.Is(err, ErrStudyDateChanged) {
		httputil.WriteError(w, 409, "ERR_STUDY_DATE_CHANGED", "学习日期已变化，请刷新当天任务", "")
		return
	}
	if errors.Is(err, ErrStudyTaskNotFound) {
		httputil.WriteError(w, 404, "ERR_STUDY_TASK_NOT_FOUND", "当天没有这项学习任务", "")
		return
	}
	if errors.Is(err, ErrAutomaticStudyTask) {
		httputil.WriteError(w, 409, "ERR_AUTOMATIC_STUDY_TASK", "练习记录已达标并自动打卡", "")
		return
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		h.fail(w, fmt.Errorf("save study task completion: %w", err))
		return
	}
	slog.Debug("study task completion saved", "user_id", uid, "study_date", today.Date, "slot_id", slotID, "task_id", taskID, "completed", *input.Completed)
	h.today(w, r)
}
