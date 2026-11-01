package plan

import (
	"errors"
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
)

// fakeStatsProvider is a StatsProvider stub returning canned stats or an error.
type fakeStatsProvider struct {
	stats *user.UserStats
	err   error
	calls int
}

func (f *fakeStatsProvider) GetStats(userID int64) (*user.UserStats, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.stats, nil
}

// errTestStoreDown stands in for a failing stats provider.
var errTestStoreDown = errors.New("db down")

func statsWith(modules map[string]user.ModuleStat) *user.UserStats {
	return &user.UserStats{StreakDays: 3, ModuleStats: modules}
}

func fixedNow() time.Time {
	// 2026-10-28 21:30 UTC == 2026-10-29 05:30 Asia/Shanghai
	return time.Date(2026, 10, 28, 21, 30, 0, 0, time.UTC)
}

func newTestService(stats *user.UserStats, err error) (*PlanService, *fakeStatsProvider) {
	provider := &fakeStatsProvider{stats: stats, err: err}
	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		panic(locErr)
	}
	svc := NewPlanService(provider, loc)
	svc.now = fixedNow
	return svc, provider
}

func TestBuildItemStatuses(t *testing.T) {
	cases := []struct {
		name      string
		completed int
		goal      int
		want      PlanItemStatus
	}{
		{"nothing done is todo", 0, 20, StatusTodo},
		{"partially done is in progress", 7, 20, StatusInProgress},
		{"exactly on goal is done", 20, 20, StatusDone},
		{"over goal is done", 31, 20, StatusDone},
		{"no goal never reports done", 5, 0, StatusInProgress},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := itemStatus(tc.completed, tc.goal); got != tc.want {
				t.Errorf("itemStatus(%d, %d) = %q, want %q", tc.completed, tc.goal, got, tc.want)
			}
		})
	}
}

func TestPercentClamps(t *testing.T) {
	cases := []struct {
		name string
		done int
		goal int
		want int
	}{
		{"zero goal avoids divide by zero", 5, 0, 0},
		{"half", 10, 20, 50},
		{"rounds down", 1, 3, 33},
		{"caps at 100", 25, 20, 100},
		{"never negative", -4, 20, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := percent(tc.done, tc.goal); got != tc.want {
				t.Errorf("percent(%d, %d) = %d, want %d", tc.done, tc.goal, got, tc.want)
			}
		})
	}
}

func TestGetDailyPlanBuildsItemsFromGoals(t *testing.T) {
	svc, provider := newTestService(statsWith(map[string]user.ModuleStat{
		"word":     {DailyGoal: 20, TodayCompleted: 4, DueCount: 12},
		"grammar":  {DailyGoal: 5, TodayCompleted: 5, DueCount: 2},
		"speaking": {DailyGoal: 3, TodayCompleted: 0, DueCount: 0},
		"writing":  {DailyGoal: 3, TodayCompleted: 1, DueCount: 0},
	}), nil)

	got, err := svc.GetDailyPlan(7)
	if err != nil {
		t.Fatalf("GetDailyPlan error: %v", err)
	}
	if provider.calls != 1 {
		t.Errorf("GetStats called %d times, want 1", provider.calls)
	}

	if got.Date != "2026-10-29" {
		t.Errorf("Date = %q, want 2026-10-29 (local date in Asia/Shanghai)", got.Date)
	}
	if got.Timezone != "Asia/Shanghai" {
		t.Errorf("Timezone = %q, want Asia/Shanghai", got.Timezone)
	}
	if got.StreakDays != 3 {
		t.Errorf("StreakDays = %d, want 3", got.StreakDays)
	}
	if got.TotalGoal != 31 {
		t.Errorf("TotalGoal = %d, want 31", got.TotalGoal)
	}
	if got.TotalCompleted != 10 {
		t.Errorf("TotalCompleted = %d, want 10", got.TotalCompleted)
	}
	if got.Remaining != 21 {
		t.Errorf("Remaining = %d, want 21", got.Remaining)
	}
	if got.Completed {
		t.Error("Completed = true, want false while goals remain")
	}

	// Started-but-unfinished work leads, untouched modules follow, and
	// finished modules sink. Canonical module order holds inside each group.
	wantOrder := []ModuleKey{ModuleWord, ModuleWriting, ModuleSpeaking, ModuleGrammar}
	if len(got.Items) != len(wantOrder) {
		t.Fatalf("Items length = %d, want %d", len(got.Items), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got.Items[i].Key != want {
			t.Errorf("Items[%d].Key = %q, want %q", i, got.Items[i].Key, want)
		}
	}

	word := got.Items[0]
	if word.Status != StatusInProgress || word.Remaining != 16 || word.DueCount != 12 {
		t.Errorf("word item = %+v, want in_progress/16 remaining/12 due", word)
	}
	if word.Path != "/words/review" {
		t.Errorf("word Path = %q, want /words/review", word.Path)
	}
	if got.Items[3].Status != StatusDone || got.Items[3].Percent != 100 {
		t.Errorf("grammar item = %+v, want done/100%%", got.Items[3])
	}
}

func TestGetDailyPlanSkipsModulesWithoutGoal(t *testing.T) {
	svc, _ := newTestService(statsWith(map[string]user.ModuleStat{
		"word":        {DailyGoal: 20, TodayCompleted: 20},
		"grammar":     {DailyGoal: 0, TodayCompleted: 9},
		"translation": {DailyGoal: 10, TodayCompleted: 1},
	}), nil)

	got, err := svc.GetDailyPlan(1)
	if err != nil {
		t.Fatalf("GetDailyPlan error: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items length = %d, want 1 (goal-less and non-plan modules skipped)", len(got.Items))
	}
	if got.Items[0].Key != ModuleWord {
		t.Errorf("Items[0].Key = %q, want word", got.Items[0].Key)
	}
	if !got.Completed {
		t.Error("Completed = false, want true when every goal is met")
	}
	if got.Percent != 100 {
		t.Errorf("Percent = %d, want 100", got.Percent)
	}
	if got.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", got.Remaining)
	}
}

func TestGetDailyPlanPropagatesStoreError(t *testing.T) {
	boom := errors.New("db down")
	svc, _ := newTestService(nil, boom)

	got, err := svc.GetDailyPlan(1)
	if err == nil {
		t.Fatal("GetDailyPlan error = nil, want wrapped store error")
	}
	if got != nil {
		t.Errorf("plan = %+v, want nil", got)
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
}

func TestGetDailyPlanRejectsNilStats(t *testing.T) {
	svc, _ := newTestService(nil, nil)

	if _, err := svc.GetDailyPlan(1); err == nil {
		t.Fatal("GetDailyPlan error = nil, want error for nil stats")
	}
}
