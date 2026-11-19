package studyplan

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"japanese-learning-app/internal/httputil"
	"japanese-learning-app/internal/module/user"
)

type Handler struct {
	db  *sql.DB
	now func() time.Time
}

func NewHandler(db *sql.DB) *Handler { return &Handler{db: db, now: time.Now} }
func (h *Handler) RegisterRoutes(protected, public *http.ServeMux) {
	protected.HandleFunc("GET /api/v1/study-plan", h.get)
	protected.HandleFunc("GET /api/v1/study-plan/today", h.today)
	protected.HandleFunc("PUT /api/v1/study-plan/tasks/{slotID}/{taskID}", h.completeTask)
	protected.HandleFunc("PUT /api/v1/study-plan", h.save)
	protected.HandleFunc("DELETE /api/v1/study-plan", h.remove)
	protected.HandleFunc("POST /api/v1/study-plan/calendar-token", h.rotate)
	public.HandleFunc("GET /api/v1/calendar/{token}", h.feed)
}
func (h *Handler) fail(w http.ResponseWriter, err error) {
	slog.Error("study plan request failed", "err", err)
	httputil.WriteError(w, 500, "ERR_INTERNAL", "学习计划操作失败", "")
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	uid, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, 401, "ERR_UNAUTHORIZED", "请先登录", "")
		return
	}
	var raw, token string
	var revision int
	err := h.db.QueryRowContext(r.Context(), `SELECT plan_json,calendar_token,revision FROM study_plans WHERE user_id=$1`, uid).Scan(&raw, &token, &revision)
	w.Header().Set("Cache-Control", "no-store")
	if errors.Is(err, sql.ErrNoRows) {
		httputil.WriteJSON(w, 200, httputil.APIResponse{Data: map[string]any{"plan": nil, "calendar_path": ""}})
		return
	}
	if err != nil {
		h.fail(w, fmt.Errorf("read study plan: %w", err))
		return
	}
	var p Plan
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		h.fail(w, fmt.Errorf("decode stored study plan: %w", err))
		return
	}
	p.Revision = revision
	httputil.WriteJSON(w, 200, httputil.APIResponse{Data: map[string]any{"plan": p, "calendar_path": "/api/v1/calendar/" + token + ".ics"}})
}
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("create calendar token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	uid, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, 401, "ERR_UNAUTHORIZED", "请先登录", "")
		return
	}
	var p Plan
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		httputil.WriteError(w, 400, "ERR_BAD_REQUEST", "计划格式无效", "")
		return
	}
	if err := p.Validate(); err != nil {
		httputil.WriteError(w, 400, "ERR_BAD_REQUEST", err.Error(), "")
		return
	}
	token, err := newToken()
	if err != nil {
		h.fail(w, err)
		return
	}
	p.Title = strings.TrimSpace(p.Title)
	p.UpdatedAt = h.now().UTC().Format(time.RFC3339)
	p.Revision = 0
	raw, err := json.Marshal(p)
	if err != nil {
		h.fail(w, fmt.Errorf("encode study plan: %w", err))
		return
	}
	// Increment revisions atomically, including multiple updates in the same second.
	_, err = h.db.ExecContext(r.Context(), `INSERT INTO study_plans(user_id,plan_json,calendar_token) VALUES($1,$2,$3) ON CONFLICT(user_id) DO UPDATE SET plan_json=excluded.plan_json,revision=study_plans.revision+1`, uid, string(raw), token)
	if err != nil {
		h.fail(w, fmt.Errorf("save study plan: %w", err))
		return
	}
	slog.Debug("study plan saved", "user_id", uid)
	h.get(w, r)
}
func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	uid, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, 401, "ERR_UNAUTHORIZED", "请先登录", "")
		return
	}
	if _, err := h.db.ExecContext(r.Context(), `DELETE FROM study_plans WHERE user_id=$1`, uid); err != nil {
		h.fail(w, fmt.Errorf("delete study plan: %w", err))
		return
	}
	w.WriteHeader(204)
}
func (h *Handler) rotate(w http.ResponseWriter, r *http.Request) {
	uid, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, 401, "ERR_UNAUTHORIZED", "请先登录", "")
		return
	}
	token, err := newToken()
	if err != nil {
		h.fail(w, err)
		return
	}
	if _, err := h.db.ExecContext(r.Context(), `UPDATE study_plans SET calendar_token=$1 WHERE user_id=$2`, token, uid); err != nil {
		h.fail(w, fmt.Errorf("reset calendar token: %w", err))
		return
	}
	h.get(w, r)
}
func (h *Handler) feed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	token := strings.TrimSuffix(r.PathValue("token"), ".ics")
	if len(token) != 64 {
		http.NotFound(w, r)
		return
	}
	var raw string
	var uid int64
	var revision int
	err := h.db.QueryRowContext(r.Context(), `SELECT user_id,plan_json,revision FROM study_plans WHERE calendar_token=$1`, token).Scan(&uid, &raw, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, fmt.Errorf("read calendar feed: %w", err))
		return
	}
	var p Plan
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		h.fail(w, fmt.Errorf("decode calendar plan: %w", err))
		return
	}
	p.Revision = revision
	content, err := Calendar(p, uid)
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="study-plan.ics"`)
	if _, err := w.Write([]byte(content)); err != nil {
		slog.Error("write calendar feed failed", "err", err)
	}
}

func calendarText(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\r\n", "\\n", "\n", "\\n", "\r", "\\n", ";", "\\;", ",", "\\,").Replace(s)
}
func Calendar(p Plan, uid int64) (string, error) {
	sessions, err := p.calendarSessions()
	if err != nil {
		return "", fmt.Errorf("generate study calendar: %w", err)
	}
	updated, err := time.Parse(time.RFC3339, p.UpdatedAt)
	if err != nil {
		return "", fmt.Errorf("parse calendar update time: %w", err)
	}
	revision := p.Revision
	if revision == 0 {
		revision = int(updated.Unix() - 1704067200)
	}
	lines := []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Japanese Learning App//Study Plan//ZH", "CALSCALE:GREGORIAN", "X-WR-CALNAME:日语学习计划", "REFRESH-INTERVAL;VALUE=DURATION:PT1H"}
	goals := []string{}
	for _, goal := range p.goals() {
		if goal.count > 0 {
			goals = append(goals, fmt.Sprintf("%s %d %s", goal.name, goal.count, goal.unit))
		}
	}
	for _, session := range sessions {
		start := session.start
		uidText := fmt.Sprintf("%d-%s", uid, start.Format("2006-01-02"))
		summary := p.Title
		description := fmt.Sprintf("今日目标：%s。学习 %d 分钟。", strings.Join(goals, "、"), session.duration)
		if session.slotID != "" {
			uidText += "-" + session.slotID
			summary += " · " + session.title
			tasks := make([]string, 0, len(session.tasks))
			for _, task := range session.tasks {
				text := fmt.Sprintf("%s（%d 分钟）：%s", task.Title, task.Minutes, task.Description)
				if task.Path != "" {
					text += " " + task.Path
				}
				tasks = append(tasks, text)
			}
			description = fmt.Sprintf("%s，学习 %d 分钟。\n%s", session.title, session.duration, strings.Join(tasks, "\n"))
		}
		lines = append(lines, "BEGIN:VEVENT", "UID:"+uidText+"@japanese-learning-app", "DTSTAMP:"+updated.UTC().Format("20060102T150405Z"), "LAST-MODIFIED:"+updated.UTC().Format("20060102T150405Z"), fmt.Sprintf("SEQUENCE:%d", revision), "DTSTART:"+start.UTC().Format("20060102T150405Z"), "DTEND:"+start.Add(time.Duration(session.duration)*time.Minute).UTC().Format("20060102T150405Z"), "SUMMARY:"+calendarText(summary), "DESCRIPTION:"+calendarText(description), "BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:"+calendarText(summary), fmt.Sprintf("TRIGGER:-PT%dM", p.Reminder), "END:VALARM", "END:VEVENT")
	}

	lines = append(lines, "END:VCALENDAR")
	var b strings.Builder
	for _, line := range lines {
		limit := 75
		for len(line) > limit {
			n := limit
			for n > 0 && !utf8.RuneStart(line[n]) {
				n--
			}
			b.WriteString(line[:n])
			b.WriteString("\r\n ")
			line = line[n:]
			limit = 74
		}
		b.WriteString(line)
		b.WriteString("\r\n")
	}
	return b.String(), nil
}
