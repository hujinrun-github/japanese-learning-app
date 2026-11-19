package studyplan

import (
	"strings"
	"testing"
	"time"
)

func slottedPlan() Plan {
	p := validPlan()
	p.StartDate, p.EndDate = "2026-11-13", "2026-12-13"
	p.TimeSlots = []TimeSlot{
		{ID: "lunch", Title: "午间词汇", Time: "12:50", Duration: 20, Tasks: []Task{{ID: "words", Title: "复习词汇", Description: "先复习到期词", Path: "/words/review?level=N2", Minutes: 20}}},
		{ID: "evening", Title: "傍晚听力", Time: "18:40", Duration: 20, Tasks: []Task{{ID: "listening", Title: "听力精练", Path: "https://www.jlpt.jp/e/samples/sampleindex.html", Minutes: 20}}},
		{ID: "night", Title: "晚间学习", Time: "22:00", Duration: 60, Tasks: []Task{
			{ID: "grammar", Title: "新语法", Path: "/grammar?level=N2", Minutes: 20, Weekdays: []int{2, 3, 4}},
			{ID: "catchup", Title: "上周补漏", Path: "/mistakes", Minutes: 20, Weekdays: []int{1}},
			{ID: "reading", Title: "阅读精读", Path: "/lesson?level=N2", Minutes: 30, Weekdays: []int{1, 2, 3, 4}},
			{ID: "review", Title: "错题复盘", Path: "/mistakes", Minutes: 10, Weekdays: []int{1, 2, 3, 4}},
			{ID: "quiz", Title: "限时测验", Path: "/grammar?level=N2", Minutes: 45, Weekdays: []int{5}},
			{ID: "analysis", Title: "测验分析", Path: "/mistakes", Minutes: 15, Weekdays: []int{5}},
		}},
	}
	return p
}

func TestValidateTimeSlots(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Plan)
		bad    bool
	}{
		{"valid", func(p *Plan) {}, false},
		{"listening catalogue", func(p *Plan) { p.TimeSlots[1].Tasks[0].Path = "/listening?level=N2" }, false},
		{"listening detail", func(p *Plan) { p.TimeSlots[1].Tasks[0].Path = "/listening/12" }, false},
		{"listening zero id", func(p *Plan) { p.TimeSlots[1].Tasks[0].Path = "/listening/0" }, true},
		{"listening nested path", func(p *Plan) { p.TimeSlots[1].Tasks[0].Path = "/listening/12/other" }, true},
		{"listening traversal", func(p *Plan) { p.TimeSlots[1].Tasks[0].Path = "/listening/%2e%2e" }, true},
		{"tasks replace legacy targets", func(p *Plan) { p.Words, p.Grammar = 0, 0 }, false},
		{"duplicate slot", func(p *Plan) { p.TimeSlots[1].ID = p.TimeSlots[0].ID }, true},
		{"unsafe slot id", func(p *Plan) { p.TimeSlots[0].ID = "../night" }, true},
		{"invalid time", func(p *Plan) { p.TimeSlots[0].Time = "25:00" }, true},
		{"overlap", func(p *Plan) { p.TimeSlots[1].Time = "13:00" }, true},
		{"non canonical time", func(p *Plan) { p.TimeSlots = p.TimeSlots[:1]; p.TimeSlots[0].Time = "9:00" }, true},
		{"past midnight", func(p *Plan) { p.TimeSlots[2].Time = "23:30" }, true},
		{"empty task list", func(p *Plan) { p.TimeSlots[0].Tasks = nil }, true},
		{"duplicate task", func(p *Plan) { p.TimeSlots[2].Tasks[1].ID = p.TimeSlots[2].Tasks[0].ID }, true},
		{"zero minutes", func(p *Plan) { p.TimeSlots[0].Tasks[0].Minutes = 0 }, true},
		{"task exceeds slot", func(p *Plan) { p.TimeSlots[0].Tasks[0].Minutes = 21 }, true},
		{"duplicate task days", func(p *Plan) { p.TimeSlots[0].Tasks[0].Weekdays = []int{1, 1} }, true},
		{"invalid task day", func(p *Plan) { p.TimeSlots[0].Tasks[0].Weekdays = []int{7} }, true},
		{"task outside plan days retained", func(p *Plan) { p.TimeSlots[0].Tasks[0].Weekdays = []int{6} }, false},
		{"negative task day", func(p *Plan) { p.TimeSlots[0].Tasks[0].Weekdays = []int{-1} }, true},
		{"unsafe external path", func(p *Plan) { p.TimeSlots[0].Tasks[0].Path = "https://evil.example" }, true},
		{"protocol relative path", func(p *Plan) { p.TimeSlots[0].Tasks[0].Path = "//evil.example" }, true},
		{"unknown internal path", func(p *Plan) { p.TimeSlots[0].Tasks[0].Path = "/admin" }, true},
		{"long description", func(p *Plan) { p.TimeSlots[0].Tasks[0].Description = strings.Repeat("x", 2001) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := slottedPlan()
			tc.change(&p)
			if err := p.Validate(); (err != nil) != tc.bad {
				t.Fatalf("Validate() = %v, bad = %v", err, tc.bad)
			}
		})
	}
}

func TestTodayResolvesOnlyLegacyListeningPath(t *testing.T) {
	p := slottedPlan()
	p.TimeSlots[2].Tasks[0].Path = "https://www.jlpt.jp/e/samples/sampleindex.html"
	today, err := p.Today(time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := today.Slots[1].Tasks[0].Path; got != "/listening?level=N2" {
		t.Fatalf("listening path = %q", got)
	}
	if got := today.Slots[2].Tasks[0].Path; got != "https://www.jlpt.jp/e/samples/sampleindex.html" {
		t.Fatalf("unrelated path changed: %q", got)
	}
	if got := p.TimeSlots[1].Tasks[0].Path; got != "https://www.jlpt.jp/e/samples/sampleindex.html" {
		t.Fatalf("stored plan mutated: %q", got)
	}
}

func TestTodayUsesPlanTimezoneAndWeekday(t *testing.T) {
	for _, tc := range []struct {
		name, now, reason     string
		slots, tasks, minutes int
		firstNight            string
	}{
		{"monday", "2026-11-16T04:00:00Z", "study_day", 3, 5, 100, "catchup"},
		{"tuesday", "2026-11-17T04:00:00Z", "study_day", 3, 5, 100, "grammar"},
		{"friday", "2026-11-20T04:00:00Z", "study_day", 3, 4, 100, "quiz"},
		{"saturday in plan zone", "2026-11-20T16:10:00Z", "rest_day", 0, 0, 0, ""},
		{"sunday", "2026-11-22T04:00:00Z", "rest_day", 0, 0, 0, ""},
		{"before start", "2026-11-12T04:00:00Z", "before_start", 0, 0, 0, ""},
		{"after end", "2026-12-14T04:00:00Z", "ended", 0, 0, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := slottedPlan()
			now, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.Today(now)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, slot := range got.Slots {
				count += len(slot.Tasks)
			}
			if got.Reason != tc.reason || len(got.Slots) != tc.slots || count != tc.tasks || got.TotalMinutes != tc.minutes {
				t.Fatalf("Today() = %+v", got)
			}
			if tc.firstNight != "" && got.Slots[2].Tasks[0].ID != tc.firstNight {
				t.Fatalf("night task = %s", got.Slots[2].Tasks[0].ID)
			}
			if got.Timezone != "Asia/Shanghai" || got.HasPlan != true {
				t.Fatalf("bad metadata: %+v", got)
			}
		})
	}
}

func TestSlottedCalendar(t *testing.T) {
	p := slottedPlan()
	p.StartDate, p.EndDate = "2026-11-20", "2026-11-23"
	p.UpdatedAt = "2026-11-19T00:00:00Z"
	content, err := Calendar(p, 7)
	if err != nil {
		t.Fatal(err)
	}
	content = strings.ReplaceAll(content, "\r\n ", "")
	if strings.Count(content, "BEGIN:VEVENT") != 6 {
		t.Fatalf("expected 6 events: %s", content)
	}
	for _, want := range []string{"UID:7-2026-11-20-lunch@japanese-learning-app", "UID:7-2026-11-20-evening@japanese-learning-app", "UID:7-2026-11-20-night@japanese-learning-app", "DTSTART:20261120T045000Z", "DTEND:20261120T051000Z", "限时测验", "上周补漏"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(content, "2026-11-21") || strings.Contains(content, "2026-11-22") {
		t.Fatal("weekend events emitted")
	}
	p.TimeSlots[0], p.TimeSlots[1] = p.TimeSlots[1], p.TimeSlots[0]
	reordered, err := Calendar(p, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reordered, "UID:7-2026-11-20-lunch@japanese-learning-app") {
		t.Fatal("reordering changed stable uid")
	}
}

func TestLegacyTodayCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		day    int
		reason string
		rest   bool
	}{
		{"weekday", 16, "legacy", false},
		{"weekend", 15, "rest_day", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validPlan().Today(time.Date(2026, 11, tc.day, 4, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			if got.Reason != tc.reason || got.RestDay != tc.rest || len(got.Slots) != 0 {
				t.Fatalf("legacy Today() = %+v", got)
			}
		})
	}
}

func TestSlottedSessionsPreserveWallClockAcrossDST(t *testing.T) {
	p := slottedPlan()
	p.StartDate, p.EndDate, p.Timezone = "2026-03-06", "2026-03-09", "America/New_York"
	sessions, err := p.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 6 {
		t.Fatalf("sessions = %v", sessions)
	}
	for i, want := range []string{"2026-03-06 12:50", "2026-03-06 18:40", "2026-03-06 22:00", "2026-03-09 12:50", "2026-03-09 18:40", "2026-03-09 22:00"} {
		if got := sessions[i].Format("2006-01-02 15:04"); got != want {
			t.Errorf("session[%d] = %s, want %s", i, got, want)
		}
	}
	if got := sessions[3].Sub(sessions[0]); got != 71*time.Hour {
		t.Fatalf("DST interval = %v, want 71h", got)
	}
}

func TestPlanWeekdaysMaskPreservesTaskSchedule(t *testing.T) {
	p := slottedPlan()
	p.Weekdays = []int{2, 3, 4, 5}
	if err := p.Validate(); err != nil {
		t.Fatalf("disabling Monday must preserve tasks: %v", err)
	}
	monday := time.Date(2026, 11, 16, 4, 0, 0, 0, time.UTC)
	today, err := p.Today(monday)
	if err != nil {
		t.Fatal(err)
	}
	if !today.RestDay || len(today.Slots) != 0 {
		t.Fatalf("disabled Monday = %+v", today)
	}
	p.Weekdays = []int{1, 2, 3, 4, 5}
	today, err = p.Today(monday)
	if err != nil {
		t.Fatal(err)
	}
	if today.TotalMinutes != 100 || len(today.Slots) != 3 || today.Slots[2].Tasks[0].ID != "catchup" {
		t.Fatalf("reenabled Monday lost tasks: %+v", today)
	}
}

func TestPlanWeekdaysMaskSuppressesWeekendTasks(t *testing.T) {
	p := slottedPlan()
	p.StartDate, p.EndDate = "2026-11-20", "2026-11-23"
	p.UpdatedAt = "2026-11-19T00:00:00Z"
	p.TimeSlots[2].Tasks = append(p.TimeSlots[2].Tasks, Task{ID: "weekend", Title: "暂未开启的周末任务", Path: "/mistakes", Minutes: 10, Weekdays: []int{0, 6}})
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, day := range []int{21, 22} {
		today, err := p.Today(time.Date(2026, 11, day, 4, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		if !today.RestDay || len(today.Slots) != 0 {
			t.Fatalf("weekend tasks escaped mask: %+v", today)
		}
	}
	calendar, err := Calendar(p, 7)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(calendar, "BEGIN:VEVENT") != 6 || strings.Contains(calendar, "weekend") || strings.Contains(calendar, "暂未开启") {
		t.Fatalf("weekend calendar tasks escaped mask: %s", calendar)
	}
}
