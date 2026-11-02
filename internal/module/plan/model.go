// Package plan builds the learner's cross-module daily study plan.
//
// The plan is computed on demand from the per-module daily goals and the
// day's real activity counters. Nothing is persisted: there is no plan table
// and no day-rollover job, so a plan can never drift out of sync with the
// underlying records.
package plan

import "time"

// ModuleKey identifies a learning module that participates in the daily plan.
type ModuleKey string

const (
	ModuleWord        ModuleKey = "word"
	ModuleGrammar     ModuleKey = "grammar"
	ModuleSpeaking    ModuleKey = "speaking"
	ModuleWriting     ModuleKey = "writing"
	ModuleTranslation ModuleKey = "translation"
)

// PlanItemStatus is the completion state of a single plan item.
type PlanItemStatus string

const (
	// StatusDone means the learner reached (or exceeded) today's goal.
	StatusDone PlanItemStatus = "done"
	// StatusInProgress means at least one unit was finished but the goal is not met.
	StatusInProgress PlanItemStatus = "in_progress"
	// StatusTodo means nothing has been finished for this module today.
	StatusTodo PlanItemStatus = "todo"
)

// moduleDescriptor keeps the presentation-agnostic wiring for a module:
// where the learner should go to work on it and in which order it appears.
type moduleDescriptor struct {
	Key  ModuleKey
	Path string
}

// moduleDescriptors defines the canonical daily-plan module set and ordering.
// Only modules that have a daily goal configured are emitted.
var moduleDescriptors = []moduleDescriptor{
	{Key: ModuleWord, Path: "/words/review"},
	{Key: ModuleGrammar, Path: "/grammar"},
	{Key: ModuleSpeaking, Path: "/speaking"},
	{Key: ModuleWriting, Path: "/writing"},
	{Key: ModuleTranslation, Path: "/translation"},
}

// DailyPlanItem is one module's slice of today's plan.
type DailyPlanItem struct {
	Key       ModuleKey      `json:"key"`
	Path      string         `json:"path"`
	Goal      int            `json:"goal"`
	Completed int            `json:"completed"`
	Remaining int            `json:"remaining"`
	DueCount  int            `json:"due_count"`
	Status    PlanItemStatus `json:"status"`
	Percent   int            `json:"percent"`
}

// DailyPlan is the learner's plan for a single calendar day.
type DailyPlan struct {
	Date           string          `json:"date"`
	Timezone       string          `json:"timezone"`
	StreakDays     int             `json:"streak_days"`
	TotalGoal      int             `json:"total_goal"`
	TotalCompleted int             `json:"total_completed"`
	Remaining      int             `json:"remaining"`
	Percent        int             `json:"percent"`
	Completed      bool            `json:"completed"`
	Items          []DailyPlanItem `json:"items"`
}

// planDay is the resolved "today" used to stamp the plan.
type planDay struct {
	date     string
	timezone string
}

func resolvePlanDay(now time.Time, loc *time.Location) planDay {
	if loc == nil {
		loc = time.Local
	}
	local := now.In(loc)
	return planDay{
		date:     local.Format("2006-01-02"),
		timezone: loc.String(),
	}
}
