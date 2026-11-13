package studyplan

import (
	"testing"
	"time"
)

func validPlan() Plan {
	return Plan{Title: "日语学习", StartDate: "2026-11-13", EndDate: "2026-12-13", Time: "20:00", Timezone: "Asia/Shanghai", Duration: 30, Reminder: 10, Weekdays: []int{1, 2, 3, 4, 5}, Words: 20, Grammar: 2}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Plan)
		bad    bool
	}{
		{"valid", func(p *Plan) {}, false},
		{"invalid date", func(p *Plan) { p.StartDate = "2026-02-30" }, true},
		{"reversed range", func(p *Plan) { p.EndDate = "2026-01-01" }, true},
		{"too long", func(p *Plan) { p.EndDate = "2030-01-01" }, true},
		{"invalid time", func(p *Plan) { p.Time = "25:00" }, true},
		{"invalid zone", func(p *Plan) { p.Timezone = "invalid" }, true},
		{"empty weekdays", func(p *Plan) { p.Weekdays = nil }, true},
		{"duplicate weekdays", func(p *Plan) { p.Weekdays = []int{1, 1} }, true},
		{"invalid weekday", func(p *Plan) { p.Weekdays = []int{7} }, true},
		{"negative target", func(p *Plan) { p.Words = -1 }, true},
		{"negative reading", func(p *Plan) { p.Reading = -1 }, true},
		{"excessive speaking", func(p *Plan) { p.Speaking = 101 }, true},
		{"negative writing", func(p *Plan) { p.Writing = -1 }, true},
		{"excessive translation", func(p *Plan) { p.Translation = 101 }, true},
		{"no targets", func(p *Plan) { p.Words = 0; p.Grammar = 0 }, true},
		{"zero duration", func(p *Plan) { p.Duration = 0 }, true},
		{"negative alert", func(p *Plan) { p.Reminder = -1 }, true},
		{"blank title", func(p *Plan) { p.Title = "  " }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := validPlan()
			tc.change(&p)
			if err := p.Validate(); (err != nil) != tc.bad {
				t.Fatalf("Validate()=%v", err)
			}
		})
	}
}

func TestSessions(t *testing.T) {
	p := validPlan()
	p.StartDate = "2026-11-13"
	p.EndDate = "2026-11-16"
	dates, err := p.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 2 || dates[0].UTC().Format(time.RFC3339) != "2026-11-13T12:00:00Z" || dates[1].Day() != 16 {
		t.Fatalf("unexpected sessions: %v", dates)
	}
	p.StartDate = "2026-03-07"
	p.EndDate = "2026-03-09"
	p.Timezone = "America/New_York"
	p.Weekdays = []int{0, 1, 6}
	dates, err = p.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 3 || dates[1].Hour() != 20 || dates[1].Sub(dates[0]) != 23*time.Hour {
		t.Fatalf("DST sessions: %v", dates)
	}
}

func TestSessionsAfterSkippedDSTTime(t *testing.T) {
	p := validPlan()
	p.StartDate, p.EndDate, p.Time, p.Timezone = "2026-03-08", "2026-03-09", "02:30", "America/New_York"
	p.Weekdays = []int{0, 1}
	sessions, err := p.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[1].Hour() != 2 || sessions[1].Minute() != 30 {
		t.Fatalf("DST normalization must not shift following days: %v", sessions)
	}
}
