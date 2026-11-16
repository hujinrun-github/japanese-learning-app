// Package studyplan manages a user's recurring learning schedule.
package studyplan

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
)

type Plan struct {
	Title       string `json:"title"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Time        string `json:"time"`
	Timezone    string `json:"timezone"`
	Duration    int    `json:"duration_minutes"`
	Reminder    int    `json:"reminder_minutes"`
	Weekdays    []int  `json:"weekdays"` // Sunday=0
	Words       int    `json:"words"`
	Grammar     int    `json:"grammar"`
	Reading     int    `json:"reading"`
	Speaking    int    `json:"speaking"`
	Writing     int    `json:"writing"`
	Translation int    `json:"translation"`
	UpdatedAt   string `json:"updated_at"`
	Revision    int    `json:"revision"`
}

func (p Plan) Validate() error {
	if strings.TrimSpace(p.Title) == "" || len(p.Title) > 240 || strings.ContainsAny(p.Title, "\r\n") {
		return fmt.Errorf("标题须为 1–240 字节且不能换行")
	}
	start, err := time.Parse("2006-01-02", p.StartDate)
	if err != nil {
		return fmt.Errorf("开始日期无效: %w", err)
	}
	end, err := time.Parse("2006-01-02", p.EndDate)
	if err != nil {
		return fmt.Errorf("结束日期无效: %w", err)
	}
	if end.Before(start) || end.Sub(start) > 366*24*time.Hour {
		return fmt.Errorf("计划日期范围须在一年内且结束不早于开始")
	}
	if _, err := time.Parse("15:04", p.Time); err != nil {
		return fmt.Errorf("学习时间无效: %w", err)
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("时区无效: %w", err)
	}
	if len(p.Weekdays) == 0 {
		return fmt.Errorf("请至少选择一个学习日")
	}
	seen := map[int]bool{}
	for _, day := range p.Weekdays {
		if day < 0 || day > 6 || seen[day] {
			return fmt.Errorf("学习日必须为不重复的 0–6")
		}
		seen[day] = true
	}
	if p.Duration < 5 || p.Duration > 480 || p.Reminder < 0 || p.Reminder > 10080 {
		return fmt.Errorf("时长须为 5–480 分钟，提前提醒须为 0–10080 分钟")
	}
	total := 0
	for _, goal := range p.goals() {
		if goal.count < 0 || goal.count > goal.max {
			return fmt.Errorf("%s目标须为 0–%d", goal.name, goal.max)
		}
		total += goal.count
	}
	if total == 0 {
		return fmt.Errorf("至少设置一项学习目标")
	}
	return nil
}

func (p Plan) Sessions() ([]time.Time, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load plan timezone: %w", err)
	}
	start, err := time.Parse("2006-01-02", p.StartDate)
	if err != nil {
		return nil, fmt.Errorf("parse plan start: %w", err)
	}
	end, err := time.Parse("2006-01-02", p.EndDate)
	if err != nil {
		return nil, fmt.Errorf("parse plan end: %w", err)
	}
	clock, err := time.Parse("15:04", p.Time)
	if err != nil {
		return nil, fmt.Errorf("parse study time: %w", err)
	}
	days := map[time.Weekday]bool{}
	for _, d := range p.Weekdays {
		days[time.Weekday(d)] = true
	}
	sessions := []time.Time{}
	// Construct each wall-clock time independently to preserve time through DST changes.
	for date := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC); !date.After(end); date = date.AddDate(0, 0, 1) {
		if days[date.Weekday()] {
			sessions = append(sessions, time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, loc))
		}
	}
	return sessions, nil
}

type studyGoal struct {
	name, unit string
	count, max int
}

func (p Plan) goals() []studyGoal {
	return []studyGoal{
		{"单词", "个", p.Words, 500},
		{"语法", "项", p.Grammar, 100},
		{"阅读理解", "篇", p.Reading, 100},
		{"口语", "次", p.Speaking, 100},
		{"写作", "篇", p.Writing, 100},
		{"翻译", "题", p.Translation, 100},
	}
}
