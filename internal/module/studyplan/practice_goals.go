package studyplan

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"japanese-learning-app/internal/data/postgres"
)

const grammarGoalReason = "grammar_goal"
const maxGrammarTarget = 100

var namedGrammarPractice = regexp.MustCompile(`^语法\s*([0-9]+)\s*(?:项|个|条)$`)
var namedGrammarDescription = regexp.MustCompile(`^学习或复习\s*[0-9]+\s*(?:项|个|条)语法[，,]\s*完成后在今日任务中打卡[。.]?$`)

type PracticeProgress struct {
	Kind         string `json:"kind"`
	Target       int    `json:"target"`
	Completed    int    `json:"completed"`
	Level        string `json:"level"`
	DueRemaining int    `json:"due_remaining"`
}

func practiceScope(path string) (kind, level string, max int) {
	if level, ok := wordReviewScope(path); ok {
		return "word_review", level, maxWordReviewTarget
	}
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || u.Path != "/grammar" {
		return "", "", 0
	}
	switch level := u.Query().Get("level"); level {
	case "N5", "N4", "N3", "N2", "N1":
		return "grammar", level, maxGrammarTarget
	case "", "all":
		return "grammar", "all", maxGrammarTarget
	default:
		return "", "", 0
	}
}

func practiceGoal(task Task, legacyGrammar int) *PracticeProgress {
	kind, level, max := practiceScope(task.Path)
	if max == 0 {
		return nil
	}
	target := 0
	switch {
	case task.PracticeTarget != nil:
		target = *task.PracticeTarget
	case kind == "word_review":
		target = wordReviewTarget(task)
	case kind == "grammar":
		target = namedPracticeTarget(namedGrammarPractice, task.Title, max)
		if target == 0 && task.ID == "grammar" && task.Title == "学习新语法" {
			target = legacyGrammar
		}
	}
	if target <= 0 || target > max {
		return nil
	}
	return &PracticeProgress{Kind: kind, Level: level, Target: target}
}

func (p *Plan) normalizePracticeTargets() {
	for i := range p.TimeSlots {
		for j := range p.TimeSlots[i].Tasks {
			task := &p.TimeSlots[i].Tasks[j]
			if progress := practiceGoal(*task, p.Grammar); progress != nil {
				if task.PracticeTarget == nil {
					task.PracticeTarget = &progress.Target
				}
				normalizePracticeLabels(task, progress)
			}
		}
	}
}

func normalizePracticeLabels(task *Task, progress *PracticeProgress) {
	switch progress.Kind {
	case "word_review":
		if namedWordReview.MatchString(normalizedPracticeText(task.Title)) {
			task.Title = fmt.Sprintf("复习%d个单词", progress.Target)
		}
	case "grammar":
		if namedGrammarPractice.MatchString(normalizedPracticeText(task.Title)) {
			task.Title = fmt.Sprintf("语法%d项", progress.Target)
		}
		if namedGrammarDescription.MatchString(normalizedPracticeText(task.Description)) {
			task.Description = "完成语法点的测验后计入进度。"
		}
	}
}

func normalizedPracticeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return r - '０' + '0'
		}
		if r == '　' {
			return ' '
		}
		return r
	}, strings.TrimSpace(s))
}

func namedPracticeTarget(pattern *regexp.Regexp, title string, max int) int {
	match := pattern.FindStringSubmatch(normalizedPracticeText(title))
	if len(match) != 2 {
		return 0
	}
	target, err := strconv.Atoi(match[1])
	if err != nil || target > max {
		return 0
	}
	return target
}

func attachPracticeProgress(ctx context.Context, db todayTaskDB, uid int64, now time.Time, wordTimezone *time.Location, plan Plan, today *TodayPlan) error {
	var words, grammar []*PracticeProgress
	if today.Reason == "legacy" && plan.Grammar > 0 {
		today.LegacyGrammar = &PracticeProgress{Kind: "grammar", Level: "all", Target: plan.Grammar}
		grammar = append(grammar, today.LegacyGrammar)
	}
	for i := range today.Slots {
		for j := range today.Slots[i].Tasks {
			task := &today.Slots[i].Tasks[j]
			task.Progress = practiceGoal(task.Task, plan.Grammar)
			if task.Progress == nil {
				continue
			}
			normalizePracticeLabels(&task.Task, task.Progress)
			switch task.Progress.Kind {
			case "word_review":
				task.WordReview = task.Progress
				words = append(words, task.Progress)
			case "grammar":
				grammar = append(grammar, task.Progress)
			}
		}
	}
	if len(words)+len(grammar) == 0 {
		return nil
	}
	loc, err := time.LoadLocation(today.Timezone)
	if err != nil {
		return fmt.Errorf("load practice goal timezone: %w", err)
	}
	local := now.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	if len(words) != 0 {
		if err := attachWordReviewProgress(ctx, db, uid, now, start, end, wordTimezone, words); err != nil {
			return err
		}
	}
	if len(grammar) != 0 {
		counts, err := postgres.CountGrammarPracticeByLevel(ctx, db, uid, start, end)
		if err != nil {
			return err
		}
		for _, progress := range grammar {
			progress.Completed = counts[progress.Level]
		}
	}
	return nil
}

// practiceCompletionChange compares the persisted check-in with the current
// plan. Explicit manual check-ins are kept when a quantity goal is edited.
func (task TodayTask) practiceCompletionChange() (reason string, completed, changed bool) {
	if task.Progress != nil && task.Progress.Completed >= task.Progress.Target {
		return task.Progress.Kind + "_goal", true, !task.Completed
	}
	if task.Completed && (task.CompletionReason == wordReviewGoalReason || task.CompletionReason == grammarGoalReason) {
		return task.CompletionReason, false, true
	}
	return "", false, false
}

func (h *Handler) ReconcilePracticeGoals(ctx context.Context, uid int64) error {
	_, err := h.readToday(ctx, uid)
	return err
}

func (h *Handler) updatePracticeCheckIns(ctx context.Context, uid int64, now time.Time) (TodayPlan, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return TodayPlan{}, fmt.Errorf("begin automatic practice check-in: %w", err)
	}
	defer tx.Rollback()
	today, err := h.syncPracticeCheckIns(ctx, tx, uid, now)
	if err != nil {
		return TodayPlan{}, err
	}
	if err := tx.Commit(); err != nil {
		return TodayPlan{}, fmt.Errorf("commit automatic practice check-in: %w", err)
	}
	return today, nil
}

func (h *Handler) syncPracticeCheckIns(ctx context.Context, tx *sql.Tx, uid int64, now time.Time) (TodayPlan, error) {
	today, err := h.readTodayState(ctx, tx, uid, now, true)
	if err != nil {
		return TodayPlan{}, err
	}
	for i := range today.Slots {
		slot := &today.Slots[i]
		for j := range slot.Tasks {
			task := &slot.Tasks[j]
			reason, completed, changed := task.practiceCompletionChange()
			if !changed {
				continue
			}
			if err := SetTaskCompletion(ctx, tx, uid, today.Date, slot.ID, task.ID, completed, reason, now); err != nil {
				return TodayPlan{}, fmt.Errorf("sync automatic practice check-in: %w", err)
			}
			task.Completed, task.CompletionReason = completed, ""
			if completed {
				task.CompletionReason = reason
			}
			slog.DebugContext(ctx, "practice goal check-in updated", "user_id", uid, "date", today.Date, "slot_id", slot.ID, "task_id", task.ID, "completed", completed, "reason", reason)
		}
	}
	return today, nil
}
