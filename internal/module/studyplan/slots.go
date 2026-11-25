package studyplan

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TimeSlot is one daily study period. Stable IDs keep calendar UIDs and task
// completions unchanged when a learner edits the start time or display order.
type TimeSlot struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Time     string `json:"time"`
	Duration int    `json:"duration_minutes"`
	Tasks    []Task `json:"tasks"`
}

type Task struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Path           string `json:"path"`
	Minutes        int    `json:"minutes"`
	Weekdays       []int  `json:"weekdays,omitempty"`
	PracticeTarget *int   `json:"practice_target,omitempty"`
	ReviewTarget   *int   `json:"review_target,omitempty"` // nil preserves existing named word tasks; zero is manual.
}

type TodayTask struct {
	Task
	Completed        bool              `json:"completed"`
	CompletionReason string            `json:"completion_reason,omitempty"`
	Progress         *PracticeProgress `json:"progress,omitempty"`
	WordReview       *PracticeProgress `json:"word_review,omitempty"` // Compatibility with existing learners.
}

type TodaySlot struct {
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Time     string      `json:"time"`
	Duration int         `json:"duration_minutes"`
	Tasks    []TodayTask `json:"tasks"`
}

type TodayPlan struct {
	HasPlan       bool              `json:"has_plan"`
	Date          string            `json:"date"`
	Timezone      string            `json:"timezone"`
	Title         string            `json:"title"`
	RestDay       bool              `json:"rest_day"`
	Reason        string            `json:"reason"`
	TotalMinutes  int               `json:"total_minutes"`
	Slots         []TodaySlot       `json:"slots"`
	LegacyGrammar *PracticeProgress `json:"legacy_grammar,omitempty"`
}

func safeID(id string) bool {
	if len(id) == 0 || len(id) > 40 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func validTaskPath(path string) bool {
	if path == "https://www.jlpt.jp/e/samples/sampleindex.html" {
		return true
	}
	if len(path) > 1000 || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\\\r\n") {
		return false
	}
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" {
		return false
	}
	switch u.Path {
	case "/words/review", "/grammar", "/lesson", "/shadowing", "/speaking", "/writing", "/translation", "/mistakes", "/notes", "/listening":
		return true
	default:
		if !strings.HasPrefix(u.Path, "/listening/") {
			return false
		}
		id := strings.TrimPrefix(u.Path, "/listening/")
		n, err := strconv.ParseInt(id, 10, 64)
		return err == nil && n > 0 && strconv.FormatInt(n, 10) == id
	}
}

// resolvedTask changes only the built-in legacy listening destination in the
// returned copy. Saved plans, unrelated official links and custom tasks survive.
func resolvedTask(task Task) Task {
	if task.ID == "listening" && task.Path == "https://www.jlpt.jp/e/samples/sampleindex.html" {
		task.Path = "/listening?level=N2"
	}
	return task
}

func (p Plan) validateTimeSlots() error {
	if len(p.TimeSlots) > 3 {
		return fmt.Errorf("每天最多安排三个学习时段")
	}
	ids := make(map[string]bool, len(p.TimeSlots))
	slots := append([]TimeSlot(nil), p.TimeSlots...)
	for _, slot := range slots {
		if !safeID(slot.ID) || ids[slot.ID] {
			return fmt.Errorf("时段 ID 无效或重复")
		}
		ids[slot.ID] = true
		if strings.TrimSpace(slot.Title) == "" || len(slot.Title) > 240 || strings.ContainsAny(slot.Title, "\r\n") {
			return fmt.Errorf("时段名称须为 1–240 字节且不能换行")
		}
		clock, err := time.Parse("15:04", slot.Time)
		if err != nil {
			return fmt.Errorf("时段时间无效: %w", err)
		}
		if clock.Format("15:04") != slot.Time {
			return fmt.Errorf("时段时间须为 HH:MM 格式")
		}
		if slot.Duration < 5 || slot.Duration > 480 || clock.Hour()*60+clock.Minute()+slot.Duration > 24*60 {
			return fmt.Errorf("时段时长须为 5–480 分钟且不能跨越午夜")
		}
		if len(slot.Tasks) == 0 || len(slot.Tasks) > 12 {
			return fmt.Errorf("每个时段须安排 1–12 项任务")
		}
		taskIDs := make(map[string]bool, len(slot.Tasks))
		for _, task := range slot.Tasks {
			if !safeID(task.ID) || taskIDs[task.ID] {
				return fmt.Errorf("任务 ID 无效或在时段内重复")
			}
			taskIDs[task.ID] = true
			if strings.TrimSpace(task.Title) == "" || len(task.Title) > 240 || strings.ContainsAny(task.Title, "\r\n") {
				return fmt.Errorf("任务名称须为 1–240 字节且不能换行")
			}
			if len(task.Description) > 2000 || task.Minutes < 1 || task.Minutes > slot.Duration {
				return fmt.Errorf("任务说明过长或任务时长超出时段")
			}
			if !validTaskPath(task.Path) {
				return fmt.Errorf("任务入口须为学习页面或官方问题集")
			}
			if task.ReviewTarget != nil {
				if _, ok := wordReviewScope(task.Path); !ok || *task.ReviewTarget < 0 || *task.ReviewTarget > maxWordReviewTarget {
					return fmt.Errorf("单词复习自动打卡目标须为 0–%d，且任务入口须为单词复习", maxWordReviewTarget)
				}
			}
			if task.PracticeTarget != nil {
				_, _, max := practiceScope(task.Path)
				if max == 0 || *task.PracticeTarget < 0 || *task.PracticeTarget > max {
					return fmt.Errorf("练习数量目标须在入口支持的范围内（0–%d）", max)
				}
			}
			days := make(map[int]bool, len(task.Weekdays))
			for _, day := range task.Weekdays {
				if day < 0 || day > 6 || days[day] {
					return fmt.Errorf("任务学习日须为不重复的 0–6")
				}
				days[day] = true
			}
		}
		for _, day := range p.Weekdays {
			minutes := 0
			for _, task := range slot.Tasks {
				if task.applies(day) {
					minutes += task.Minutes
				}
			}
			if minutes > slot.Duration {
				return fmt.Errorf("同一天任务总时长不能超过所在时段")
			}
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].Time < slots[j].Time })
	for i := 1; i < len(slots); i++ {
		prev, err := time.Parse("15:04", slots[i-1].Time)
		if err != nil {
			return fmt.Errorf("解析前一时段时间: %w", err)
		}
		current, err := time.Parse("15:04", slots[i].Time)
		if err != nil {
			return fmt.Errorf("解析当前时段时间: %w", err)
		}
		if prev.Add(time.Duration(slots[i-1].Duration) * time.Minute).After(current) {
			return fmt.Errorf("学习时段不能重叠")
		}
	}
	return nil
}

func (t Task) applies(day int) bool {
	if len(t.Weekdays) == 0 {
		return true
	}
	for _, weekday := range t.Weekdays {
		if weekday == day {
			return true
		}
	}
	return false
}

// Today resolves the schedule in the plan's timezone, never the browser's or
// server's timezone. Completion records are merged separately by the handler.
func (p Plan) Today(now time.Time) (TodayPlan, error) {
	if err := p.Validate(); err != nil {
		return TodayPlan{}, fmt.Errorf("resolve today's study plan: %w", err)
	}
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return TodayPlan{}, fmt.Errorf("load today's timezone: %w", err)
	}
	local := now.In(loc)
	date := local.Format("2006-01-02")
	result := TodayPlan{HasPlan: true, Date: date, Timezone: p.Timezone, Title: p.Title, Slots: []TodaySlot{}}
	switch {
	case date < p.StartDate:
		result.Reason = "before_start"
	case date > p.EndDate:
		result.Reason = "ended"
	default:
		active := false
		for _, day := range p.Weekdays {
			active = active || day == int(local.Weekday())
		}
		if !active {
			result.Reason, result.RestDay = "rest_day", true
		} else if len(p.TimeSlots) == 0 {
			result.Reason = "legacy"
		} else {
			result.Reason = "study_day"
			result.Slots = p.slotsForDay(int(local.Weekday()))
			for _, slot := range result.Slots {
				result.TotalMinutes += slot.Duration
			}
			if len(result.Slots) == 0 {
				result.Reason, result.RestDay = "rest_day", true
			}
		}
	}
	return result, nil
}

func (p Plan) slotsForDay(day int) []TodaySlot {
	slots := make([]TodaySlot, 0, len(p.TimeSlots))
	for _, slot := range p.TimeSlots {
		todaySlot := TodaySlot{ID: slot.ID, Title: slot.Title, Time: slot.Time, Duration: slot.Duration, Tasks: []TodayTask{}}
		for _, task := range slot.Tasks {
			if task.applies(day) {
				todaySlot.Tasks = append(todaySlot.Tasks, TodayTask{Task: resolvedTask(task)})
			}
		}
		if len(todaySlot.Tasks) > 0 {
			slots = append(slots, todaySlot)
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].Time < slots[j].Time })
	return slots
}

type calendarSession struct {
	start    time.Time
	slotID   string
	title    string
	duration int
	tasks    []TodayTask
}

func (p Plan) calendarSessions() ([]calendarSession, error) {
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
	var sessions []calendarSession
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		active := false
		for _, day := range p.Weekdays {
			active = active || day == int(date.Weekday())
		}
		if !active {
			continue
		}
		slots := p.slotsForDay(int(date.Weekday()))
		if len(p.TimeSlots) == 0 {
			slots = []TodaySlot{{Time: p.Time, Title: p.Title, Duration: p.Duration}}
		}
		for _, slot := range slots {
			clock, err := time.Parse("15:04", slot.Time)
			if err != nil {
				return nil, fmt.Errorf("parse session time: %w", err)
			}
			at := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
			sessions = append(sessions, calendarSession{start: at, slotID: slot.ID, title: slot.Title, duration: slot.Duration, tasks: slot.Tasks})
		}
	}
	return sessions, nil
}
