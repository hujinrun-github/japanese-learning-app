package studyplan

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"time"
)

const maxWordReviewTarget = 200
const wordReviewGoalReason = "word_review_goal"

var namedWordReview = regexp.MustCompile(`^复习\s*([0-9]+)\s*(?:个|条)?\s*(?:单词|词汇)$`)

func wordReviewScope(path string) (string, bool) {
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || u.Path != "/words/review" {
		return "", false
	}
	query := u.Query()
	if query.Has("word_id") || (query.Get("queue") != "" && query.Get("queue") != "review") {
		return "", false
	}
	switch level := query.Get("level"); level {
	case "N5", "N4", "N3", "N2", "N1", "all":
		return level, true
	case "":
		return "all", true
	}
	return "", false
}

func wordReviewTarget(task Task) int {
	if task.ReviewTarget != nil {
		return *task.ReviewTarget
	}
	// Older plans such as “复习20个单词” have the count only in their title.
	return namedPracticeTarget(namedWordReview, task.Title, maxWordReviewTarget)
}

func attachWordReviewProgress(ctx context.Context, db todayTaskDB, uid int64, now, start, end time.Time, queueLocation *time.Location, goals []*PracticeProgress) error {
	queueNow := now.In(queueLocation)
	queueStart := time.Date(queueNow.Year(), queueNow.Month(), queueNow.Day(), 0, 0, 0, 0, queueLocation)
	// Ratings use the plan date. The due count mirrors the normal review queue,
	// whose window is based on the application's configured timezone.
	rows, err := db.QueryContext(ctx, `SELECT w.jlpt_level,
		COUNT(*) FILTER (WHERE EXISTS (
		  SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(r.review_history_json) = 'array'
		    THEN r.review_history_json ELSE '[]'::jsonb END) AS ev
		  WHERE NULLIF(ev->>'reviewed_at', '')::timestamptz >= $2
		    AND NULLIF(ev->>'reviewed_at', '')::timestamptz < $3
		)), COUNT(*) FILTER (WHERE r.next_review_at <= $4 AND r.updated_at < $5)
		FROM word_records r JOIN words w ON w.id = r.word_id
		WHERE r.user_id = $1 GROUP BY w.jlpt_level`, uid, start.UTC(), end.UTC(), now.UTC(), queueStart.UTC())
	if err != nil {
		return fmt.Errorf("read planned word review counts: %w", err)
	}
	defer rows.Close()
	counts := make(map[string]int)
	dueCounts := make(map[string]int)
	for rows.Next() {
		var level string
		var count, due int
		if err := rows.Scan(&level, &count, &due); err != nil {
			return fmt.Errorf("scan planned word review count: %w", err)
		}
		counts[level] = count
		counts["all"] += count
		dueCounts[level] = due
		dueCounts["all"] += due
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate planned word review counts: %w", err)
	}
	for _, goal := range goals {
		goal.Completed = counts[goal.Level]
		goal.DueRemaining = dueCounts[goal.Level]
	}
	return nil
}
