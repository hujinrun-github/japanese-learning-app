package plan

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"japanese-learning-app/internal/module/user"
)

// StatsProvider supplies the per-module counters the plan is derived from.
// It is satisfied by *user.UserService, which already aggregates goals,
// due counts and today's completed counts in one round trip.
type StatsProvider interface {
	GetStats(userID int64) (*user.UserStats, error)
}

// PlanService assembles the daily plan from module goals and today's activity.
type PlanService struct {
	stats StatsProvider
	now   func() time.Time
	loc   *time.Location
}

// NewPlanService creates a PlanService. A nil loc falls back to time.Local,
// which keeps the plan aligned with the server's configured APP_TIMEZONE.
func NewPlanService(stats StatsProvider, loc *time.Location) *PlanService {
	if loc == nil {
		loc = time.Local
	}
	return &PlanService{stats: stats, now: time.Now, loc: loc}
}

// GetDailyPlan returns today's cross-module plan for the user.
//
// The plan is derived on every call: there is no persisted plan and no
// day-rollover job, so it can never drift from the underlying records.
func (s *PlanService) GetDailyPlan(userID int64) (*DailyPlan, error) {
	slog.Debug("PlanService.GetDailyPlan called", "user_id", userID)

	stats, err := s.stats.GetStats(userID)
	if err != nil {
		slog.Error("PlanService.GetDailyPlan: GetStats failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("plan.PlanService.GetDailyPlan: %w", err)
	}
	if stats == nil {
		return nil, fmt.Errorf("plan.PlanService.GetDailyPlan: nil stats for user %d", userID)
	}

	day := resolvePlanDay(s.now(), s.loc)
	plan := &DailyPlan{
		Date:       day.date,
		Timezone:   day.timezone,
		StreakDays: stats.StreakDays,
		Items:      []DailyPlanItem{},
	}

	for _, desc := range moduleDescriptors {
		stat, ok := stats.ModuleStats[string(desc.Key)]
		if !ok || stat.DailyGoal <= 0 {
			continue
		}
		item := buildItem(desc, stat)
		plan.Items = append(plan.Items, item)
		plan.TotalGoal += item.Goal
		plan.TotalCompleted += item.Completed
	}

	sortItems(plan.Items)

	plan.Remaining = subtractClamped(plan.TotalGoal, plan.TotalCompleted)
	plan.Percent = percent(plan.TotalCompleted, plan.TotalGoal)
	plan.Completed = plan.TotalGoal > 0 && plan.TotalCompleted >= plan.TotalGoal

	slog.Debug("PlanService.GetDailyPlan done",
		"user_id", userID,
		"date", plan.Date,
		"items", len(plan.Items),
		"goal", plan.TotalGoal,
		"completed", plan.TotalCompleted,
	)
	return plan, nil
}

// buildItem turns one module's counters into a plan item.
func buildItem(desc moduleDescriptor, stat user.ModuleStat) DailyPlanItem {
	completed := stat.TodayCompleted
	if completed < 0 {
		completed = 0
	}
	goal := stat.DailyGoal
	due := stat.DueCount
	if due < 0 {
		due = 0
	}

	return DailyPlanItem{
		Key:       desc.Key,
		Path:      desc.Path,
		Goal:      goal,
		Completed: completed,
		Remaining: subtractClamped(goal, completed),
		DueCount:  due,
		Status:    itemStatus(completed, goal),
		Percent:   percent(completed, goal),
	}
}

// itemStatus classifies a module against its daily goal.
// A module with no goal can never be reported as done.
func itemStatus(completed, goal int) PlanItemStatus {
	switch {
	case goal > 0 && completed >= goal:
		return StatusDone
	case completed > 0:
		return StatusInProgress
	default:
		return StatusTodo
	}
}

// percent returns completed/goal as a clamped 0..100 integer.
// A goal of zero yields zero rather than a divide-by-zero panic.
func percent(completed, goal int) int {
	if goal <= 0 {
		return 0
	}
	p := completed * 100 / goal
	if p > 100 {
		return 100
	}
	if p < 0 {
		return 0
	}
	return p
}

func subtractClamped(total, done int) int {
	if diff := total - done; diff > 0 {
		return diff
	}
	return 0
}

// statusRank orders work so the learner sees what to pick up next:
// finish what is already started, then start what is untouched, and let
// completed modules sink to the bottom.
func statusRank(status PlanItemStatus) int {
	switch status {
	case StatusInProgress:
		return 0
	case StatusTodo:
		return 1
	default:
		return 2
	}
}

// sortItems floats unfinished modules to the top while preserving the
// canonical module order inside each group, so the list is stable to read.
func sortItems(items []DailyPlanItem) {
	sort.SliceStable(items, func(i, j int) bool {
		return statusRank(items[i].Status) < statusRank(items[j].Status)
	})
}
