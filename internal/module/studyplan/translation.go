package studyplan

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var translationTaskTitle = regexp.MustCompile(`^(?:(?:今日|每日) *)?(?:翻译|翻訳)(?:练习|練習)? *([0-9]+) *(?:道 *)?[题題問]$`)

// ResolveTranslationTarget returns any scheduled target in the learner's plan
// and its timezone. Existing time-slot plans store the quantity in the title.
func ResolveTranslationTarget(raw string, now time.Time) (int, *time.Location, error) {
	var plan Plan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return 0, nil, fmt.Errorf("decode translation study plan: %w", err)
	}
	today, err := plan.Today(now)
	if err != nil {
		return 0, nil, err
	}
	target := 0
	if today.Reason == "legacy" {
		target = plan.Translation
	}
	for _, slot := range today.Slots {
		for _, task := range slot.Tasks {
			target += translationTaskCount(task)
		}
	}
	if target == 0 {
		return 0, nil, nil
	}
	loc, err := time.LoadLocation(today.Timezone)
	if err != nil {
		return 0, nil, fmt.Errorf("load translation study timezone: %w", err)
	}
	return target, loc, nil
}

func translationTaskCount(task TodayTask) int {
	path, _, _ := strings.Cut(task.Path, "?")
	path, _, _ = strings.Cut(path, "#")
	if path != "/translation" {
		return 0
	}
	title := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if r >= '０' && r <= '９' {
			return r - '０' + '0'
		}
		return r
	}, task.Title)
	match := translationTaskTitle.FindStringSubmatch(strings.TrimSpace(title))
	if len(match) == 0 {
		return 0
	}
	count, err := strconv.Atoi(match[1])
	if err != nil || count > 100 {
		return 0
	}
	return count
}
