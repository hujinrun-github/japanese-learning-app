# Word Review Plan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-word review plan dashboard at `/words/review-plan` with backend plan data, frontend summary, seven-day schedule, filters, and links back into the existing review flow.

**Architecture:** Keep the existing `/words/review` flashcard flow intact. Add a read-only plan API under the word module, backed by focused store queries and a `WordService.GetReviewPlan` method that groups records by review date and calculates new-word slots. The frontend adds one route and one page that fetches this API, renders dashboard data, and filters the returned items client-side.

**Tech Stack:** Go `net/http`, SQLite via existing `database/sql` stores, React 18, TypeScript, CSS modules, Vitest, Makefile commands.

---

## File Structure

- Modify `internal/module/word/model.go`: add review plan response models and status constants.
- Modify `internal/module/word/service.go`: extend `WordStoreInterface` and add `GetReviewPlan`.
- Modify `internal/module/word/service_test.go`: add table-driven service tests for date grouping and plan math.
- Modify `internal/data/word_store.go`: add store queries for plan records, new-word candidates, mastery distribution, and today completed count.
- Modify `internal/data/adapters.go`: expose new store methods through `WordStoreAdapter`.
- Modify `internal/data/word_store_test.go`: add integration-style store tests using real SQLite.
- Modify `internal/module/word/handler.go`: add `GET /api/v1/words/review-plan`.
- Modify `internal/module/word/handler_test.go` if present; otherwise create focused handler tests in `internal/module/word/handler_test.go`.
- Modify `front/react/src/types/api.ts`: add TypeScript plan types.
- Create `front/react/src/pages/word/WordReviewPlanPage.tsx`: render the review plan dashboard.
- Create `front/react/src/pages/word/WordReviewPlanPage.module.css`: page styles.
- Create `front/react/src/pages/word/reviewPlan.ts`: filter and label helpers for testable frontend behavior.
- Create `front/react/src/pages/word/__tests__/reviewPlan.test.ts`: Vitest tests for filters and status labels.
- Modify `front/react/src/App.tsx`: add `/words/review-plan` route.
- Modify `front/react/src/pages/home/HomePage.tsx`: make “查看复习计划” link to `/words/review-plan`.
- Modify `front/react/src/pages/word/WordReviewPage.tsx`: add a “查看计划” link.
- Modify `front/react/src/i18n/locales/zh.ts`, `front/react/src/i18n/locales/en.ts`, `front/react/src/i18n/locales/ja.ts`: add page copy.

---

### Task 1: Backend Models And Service Plan Calculation

**Files:**
- Modify: `internal/module/word/model.go`
- Modify: `internal/module/word/service.go`
- Test: `internal/module/word/service_test.go`

- [ ] **Step 1: Write the failing service test**

Add table-driven tests to `internal/module/word/service_test.go`. Extend the existing `fakeWordStore` with fields for `planRecords`, `newCandidates`, `newCandidateCount`, `masteryDistribution`, and `todayCompleted`.

```go
func TestWordService_GetReviewPlan(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.Local)

	tests := []struct {
		name      string
		records   []word.WordReviewPlanRecord
		newWords  []word.Word
		dailyGoal int
		want      word.ReviewPlan
	}{
		{
			name: "groups overdue today and future records with new slots",
			records: []word.WordReviewPlanRecord{
				{Word: word.Word{ID: 1, KanjiForm: "古い", Reading: "ふるい", JLPTLevel: word.LevelN5}, Record: word.WordRecord{WordID: 1, MasteryLevel: 1, NextReviewAt: now.AddDate(0, 0, -1), Interval: 1, EaseFactor: 2.5}},
				{Word: word.Word{ID: 2, KanjiForm: "今日", Reading: "きょう", JLPTLevel: word.LevelN5}, Record: word.WordRecord{WordID: 2, MasteryLevel: 2, NextReviewAt: now, Interval: 6, EaseFactor: 2.5}},
				{Word: word.Word{ID: 3, KanjiForm: "明日", Reading: "あした", JLPTLevel: word.LevelN5}, Record: word.WordRecord{WordID: 3, MasteryLevel: 3, NextReviewAt: now.AddDate(0, 0, 1), Interval: 10, EaseFactor: 2.6}},
			},
			newWords: []word.Word{{ID: 4, KanjiForm: "新語", Reading: "しんご", JLPTLevel: word.LevelN5}},
			dailyGoal: 5,
			want: word.ReviewPlan{
				Level:              word.LevelN5,
				DailyGoal:          5,
				TodayCompleted:     0,
				DueCount:           2,
				FutureDueCount:     1,
				NewCandidatesCount: 1,
			},
		},
		{
			name: "mastered records are listed as mastered and excluded from due count",
			records: []word.WordReviewPlanRecord{
				{Word: word.Word{ID: 5, KanjiForm: "卒業", Reading: "そつぎょう", JLPTLevel: word.LevelN5}, Record: word.WordRecord{WordID: 5, MasteryLevel: 5, NextReviewAt: now, Interval: 30, EaseFactor: 2.8}},
			},
			dailyGoal: 3,
			want: word.ReviewPlan{
				Level:          word.LevelN5,
				DailyGoal:      3,
				TodayCompleted: 0,
				DueCount:       0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeWordStore()
			store.planRecords = tt.records
			store.newWords = tt.newWords
			store.newCandidateCount = len(tt.newWords)
			store.todayCompleted = tt.want.TodayCompleted
			store.masteryDistribution = map[int]int{0: len(tt.newWords)}

			svc := word.NewWordService(store)
			got, err := svc.GetReviewPlan(100, word.LevelN5, 7, tt.dailyGoal, now)
			if err != nil {
				t.Fatalf("GetReviewPlan error = %v", err)
			}

			if got.Level != tt.want.Level || got.DailyGoal != tt.want.DailyGoal || got.DueCount != tt.want.DueCount || got.FutureDueCount != tt.want.FutureDueCount || got.NewCandidatesCount != tt.want.NewCandidatesCount {
				t.Fatalf("summary = level %s goal %d due %d future %d new %d, want level %s goal %d due %d future %d new %d",
					got.Level, got.DailyGoal, got.DueCount, got.FutureDueCount, got.NewCandidatesCount,
					tt.want.Level, tt.want.DailyGoal, tt.want.DueCount, tt.want.FutureDueCount, tt.want.NewCandidatesCount)
			}
			if len(got.Days) != 7 {
				t.Fatalf("Days len = %d, want 7", len(got.Days))
			}
		})
	}
}
```

- [ ] **Step 2: Run the service test and verify it fails**

Run:

```powershell
go test ./internal/module/word -run TestWordService_GetReviewPlan -count=1
```

Expected: FAIL because `WordReviewPlanRecord`, `ReviewPlan`, and `GetReviewPlan` do not exist yet.

- [ ] **Step 3: Add review plan models**

In `internal/module/word/model.go`, add:

```go
type ReviewPlanStatus string

const (
	ReviewPlanStatusOverdue  ReviewPlanStatus = "overdue"
	ReviewPlanStatusDue      ReviewPlanStatus = "due"
	ReviewPlanStatusFuture   ReviewPlanStatus = "future"
	ReviewPlanStatusMastered ReviewPlanStatus = "mastered"
	ReviewPlanStatusNew      ReviewPlanStatus = "new"
)

type WordReviewPlanRecord struct {
	Word   Word
	Record WordRecord
}

type ReviewPlan struct {
	Level              JLPTLevel          `json:"level"`
	DailyGoal          int                `json:"daily_goal"`
	TodayCompleted     int                `json:"today_completed"`
	DueCount           int                `json:"due_count"`
	NewCandidatesCount int                `json:"new_candidates_count"`
	FutureDueCount     int                `json:"future_due_count"`
	Days               []ReviewPlanDay    `json:"days"`
	MasteryDistribution map[string]int    `json:"mastery_distribution"`
	Items              []ReviewPlanItem   `json:"items"`
}

type ReviewPlanDay struct {
	Date         string `json:"date"`
	DueCount     int    `json:"due_count"`
	OverdueCount int    `json:"overdue_count"`
	NewSlots     int    `json:"new_slots"`
	TotalPlanned int    `json:"total_planned"`
}

type ReviewPlanItem struct {
	Word            Word             `json:"word"`
	Record          *WordRecord      `json:"record,omitempty"`
	Status          ReviewPlanStatus `json:"status"`
	DaysUntilReview int              `json:"days_until_review"`
}
```

- [ ] **Step 4: Extend service store interface**

In `internal/module/word/service.go`, extend `WordStoreInterface`:

```go
	ListReviewPlanRecords(userID int64, level JLPTLevel, days int) ([]WordReviewPlanRecord, error)
	ListNewWordCandidates(userID int64, level JLPTLevel, limit int) ([]Word, error)
	CountNewWordCandidates(userID int64, level JLPTLevel) (int, error)
	CountTodayCompleted(userID int64) (int, error)
	MasteryDistribution(userID int64, level JLPTLevel) (map[int]int, error)
```

- [ ] **Step 5: Implement minimal service calculation**

In `internal/module/word/service.go`, add:

```go
func (s *WordService) GetReviewPlan(userID int64, level JLPTLevel, days int, dailyGoal int, now time.Time) (*ReviewPlan, error) {
	if days < 1 {
		days = 7
	}
	if days > 30 {
		days = 30
	}
	if dailyGoal < 0 {
		dailyGoal = 0
	}
	start := startOfDay(now)

	records, err := s.store.ListReviewPlanRecords(userID, level, days)
	if err != nil {
		slog.Error("WordService.GetReviewPlan: ListReviewPlanRecords failed", "err", err, "user_id", userID, "level", level)
		return nil, fmt.Errorf("word.WordService.GetReviewPlan ListReviewPlanRecords: %w", err)
	}
	newCount, err := s.store.CountNewWordCandidates(userID, level)
	if err != nil {
		slog.Error("WordService.GetReviewPlan: CountNewWordCandidates failed", "err", err, "user_id", userID, "level", level)
		return nil, fmt.Errorf("word.WordService.GetReviewPlan CountNewWordCandidates: %w", err)
	}
	newWords, err := s.store.ListNewWordCandidates(userID, level, dailyGoal)
	if err != nil {
		slog.Error("WordService.GetReviewPlan: ListNewWordCandidates failed", "err", err, "user_id", userID, "level", level)
		return nil, fmt.Errorf("word.WordService.GetReviewPlan ListNewWordCandidates: %w", err)
	}
	todayCompleted, err := s.store.CountTodayCompleted(userID)
	if err != nil {
		slog.Error("WordService.GetReviewPlan: CountTodayCompleted failed", "err", err, "user_id", userID)
		return nil, fmt.Errorf("word.WordService.GetReviewPlan CountTodayCompleted: %w", err)
	}
	dist, err := s.store.MasteryDistribution(userID, level)
	if err != nil {
		slog.Error("WordService.GetReviewPlan: MasteryDistribution failed", "err", err, "user_id", userID, "level", level)
		return nil, fmt.Errorf("word.WordService.GetReviewPlan MasteryDistribution: %w", err)
	}

	plan := &ReviewPlan{
		Level:               level,
		DailyGoal:           dailyGoal,
		TodayCompleted:      todayCompleted,
		NewCandidatesCount:  newCount,
		Days:                make([]ReviewPlanDay, days),
		MasteryDistribution: normalizeMasteryDistribution(dist, len(newWords)),
	}
	for i := 0; i < days; i++ {
		plan.Days[i] = ReviewPlanDay{Date: start.AddDate(0, 0, i).Format("2006-01-02")}
	}

	for _, rec := range records {
		status, offset := classifyReviewPlanRecord(rec.Record, start, days)
		item := ReviewPlanItem{Word: rec.Word, Record: &rec.Record, Status: status, DaysUntilReview: offset}
		plan.Items = append(plan.Items, item)
		if status == ReviewPlanStatusMastered {
			continue
		}
		if status == ReviewPlanStatusOverdue {
			plan.Days[0].OverdueCount++
			plan.DueCount++
			continue
		}
		if status == ReviewPlanStatusDue {
			plan.Days[0].DueCount++
			plan.DueCount++
			continue
		}
		if status == ReviewPlanStatusFuture && offset >= 0 && offset < len(plan.Days) {
			plan.Days[offset].DueCount++
			plan.FutureDueCount++
		}
	}

	todayPlanned := plan.Days[0].DueCount + plan.Days[0].OverdueCount
	if dailyGoal > todayPlanned {
		plan.Days[0].NewSlots = minInt(dailyGoal-todayPlanned, newCount)
	}
	for i := range plan.Days {
		plan.Days[i].TotalPlanned = plan.Days[i].DueCount + plan.Days[i].OverdueCount + plan.Days[i].NewSlots
	}
	for _, w := range newWords {
		plan.Items = append(plan.Items, ReviewPlanItem{Word: w, Status: ReviewPlanStatusNew, DaysUntilReview: 0})
	}
	return plan, nil
}
```

Add helper functions in the same file:

```go
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func classifyReviewPlanRecord(r WordRecord, today time.Time, days int) (ReviewPlanStatus, int) {
	if r.MasteryLevel >= 5 {
		return ReviewPlanStatusMastered, 0
	}
	reviewDay := startOfDay(r.NextReviewAt.In(today.Location()))
	offset := int(reviewDay.Sub(today).Hours() / 24)
	if offset < 0 {
		return ReviewPlanStatusOverdue, offset
	}
	if offset == 0 {
		return ReviewPlanStatusDue, 0
	}
	if offset < days {
		return ReviewPlanStatusFuture, offset
	}
	return ReviewPlanStatusFuture, offset
}

func normalizeMasteryDistribution(dist map[int]int, newCount int) map[string]int {
	out := map[string]int{"0": newCount, "1": 0, "2": 0, "3": 0, "4": 0, "5": 0}
	for level, count := range dist {
		key := fmt.Sprintf("%d", level)
		if level >= 5 {
			key = "5"
		}
		out[key] += count
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 6: Run the service test and verify it passes**

Run:

```powershell
go test ./internal/module/word -run TestWordService_GetReviewPlan -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 1**

Run:

```powershell
git add -- internal/module/word/model.go internal/module/word/service.go internal/module/word/service_test.go
git commit -m "feat(word): calculate review plan"
```

---

### Task 2: Data Store Queries For Plan Data

**Files:**
- Modify: `internal/data/word_store.go`
- Modify: `internal/data/adapters.go`
- Test: `internal/data/word_store_test.go`

- [ ] **Step 1: Write failing store tests**

Add tests to `internal/data/word_store_test.go` using the existing test DB helpers in that file:

```go
func TestWordStore_ReviewPlanQueries(t *testing.T) {
	store := &WordStore{db: testDB}
	userID := int64(9010)
	_, err := testDB.Exec(
		`INSERT OR IGNORE INTO users (id, email, password_hash, goal_level) VALUES (?, ?, ?, ?)`,
		userID, "review-plan@example.com", "hash", "N5",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	wordID := int64(9101)
	futureWordID := int64(9102)
	newWordID := int64(9103)
	_, err = testDB.Exec(
		`INSERT OR IGNORE INTO words (id, kanji_form, reading, part_of_speech, meaning, examples_json, jlpt_level, reading_type, audio_url, updated_at)
		 VALUES (?, '計画', 'けいかく', '名詞', '计划', '[]', 'N5', '', '', datetime('now')),
		        (?, '未来', 'みらい', '名詞', '未来', '[]', 'N5', '', '', datetime('now')),
		        (?, '新語', 'しんご', '名詞', '新词', '[]', 'N5', '', '', datetime('now'))`,
		wordID, futureWordID, newWordID,
	)
	if err != nil {
		t.Fatalf("insert words: %v", err)
	}

	_, err = testDB.Exec(`INSERT INTO word_records
		(user_id, word_id, mastery_level, next_review_at, ease_factor, interval, review_history_json, updated_at)
		VALUES (?, ?, 2, datetime('now'), 2.5, 6, '[]', datetime('now')),
		       (?, ?, 1, datetime('now', '+2 day'), 2.4, 3, '[]', datetime('now'))`,
		userID, wordID, userID, futureWordID)
	if err != nil {
		t.Fatalf("insert word_records: %v", err)
	}

	records, err := store.ListReviewPlanRecords(userID, word.LevelN5, 7)
	if err != nil {
		t.Fatalf("ListReviewPlanRecords error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("ListReviewPlanRecords len = %d, want 2", len(records))
	}

	newCount, err := store.CountNewWordCandidates(userID, word.LevelN5)
	if err != nil {
		t.Fatalf("CountNewWordCandidates error = %v", err)
	}
	if newCount != 1 {
		t.Fatalf("CountNewWordCandidates = %d, want 1", newCount)
	}

	completed, err := store.CountTodayCompleted(userID)
	if err != nil {
		t.Fatalf("CountTodayCompleted error = %v", err)
	}
	if completed != 2 {
		t.Fatalf("CountTodayCompleted = %d, want 2", completed)
	}
}
```

- [ ] **Step 2: Run the store tests and verify they fail**

Run:

```powershell
go test ./internal/data -run TestWordStore_ReviewPlanQueries -count=1
```

Expected: FAIL because the new store methods do not exist.

- [ ] **Step 3: Implement store queries**

Add methods to `internal/data/word_store.go`:

```go
func (s *WordStore) ListReviewPlanRecords(userID int64, level word.JLPTLevel, days int) ([]word.WordReviewPlanRecord, error) {
	rows, err := s.db.Query(
		`SELECT w.id, w.kanji_form, w.reading, w.part_of_speech, w.meaning, w.examples_json, w.jlpt_level, w.reading_type, w.audio_url,
		        r.id, r.user_id, r.word_id, r.mastery_level, r.next_review_at, r.ease_factor, r.interval, r.review_history_json, r.updated_at
		   FROM word_records r
		   JOIN words w ON w.id = r.word_id
		  WHERE r.user_id = ?
		    AND w.jlpt_level = ?
		    AND (r.mastery_level >= 5 OR r.next_review_at < datetime('now', '+' || ? || ' day'))
		  ORDER BY r.next_review_at ASC`,
		userID, level, days,
	)
	if err != nil {
		slog.Error("failed to query word review plan records", "err", err, "user_id", userID, "level", level)
		return nil, fmt.Errorf("data.WordStore.ListReviewPlanRecords query: %w", err)
	}
	defer rows.Close()

	var result []word.WordReviewPlanRecord
	for rows.Next() {
		item, err := scanWordReviewPlanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("data.WordStore.ListReviewPlanRecords scan: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data.WordStore.ListReviewPlanRecords rows: %w", err)
	}
	return result, nil
}
```

Also add:

```go
func (s *WordStore) CountNewWordCandidates(userID int64, level word.JLPTLevel) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*)
		   FROM words w
		  WHERE w.jlpt_level = ?
		    AND NOT EXISTS (
		      SELECT 1 FROM word_records r
		       WHERE r.user_id = ? AND r.word_id = w.id
		    )`,
		level, userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("data.WordStore.CountNewWordCandidates: %w", err)
	}
	return count, nil
}

func (s *WordStore) ListNewWordCandidates(userID int64, level word.JLPTLevel, limit int) ([]word.Word, error) {
	rows, err := s.db.Query(
		`SELECT w.id, w.kanji_form, w.reading, w.part_of_speech, w.meaning, w.examples_json, w.jlpt_level, w.reading_type, w.audio_url
		   FROM words w
		  WHERE w.jlpt_level = ?
		    AND NOT EXISTS (
		      SELECT 1 FROM word_records r
		       WHERE r.user_id = ? AND r.word_id = w.id
		    )
		  ORDER BY w.id ASC
		  LIMIT ?`,
		level, userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("data.WordStore.ListNewWordCandidates query: %w", err)
	}
	defer rows.Close()
	return scanWords(rows, "data.WordStore.ListNewWordCandidates")
}

func (s *WordStore) CountTodayCompleted(userID int64) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM word_records
		  WHERE user_id = ? AND date(updated_at) = date('now')`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("data.WordStore.CountTodayCompleted: %w", err)
	}
	return count, nil
}

func (s *WordStore) MasteryDistribution(userID int64, level word.JLPTLevel) (map[int]int, error) {
	rows, err := s.db.Query(
		`SELECT r.mastery_level, COUNT(*)
		   FROM word_records r
		   JOIN words w ON w.id = r.word_id
		  WHERE r.user_id = ? AND w.jlpt_level = ?
		  GROUP BY r.mastery_level`,
		userID, level,
	)
	if err != nil {
		return nil, fmt.Errorf("data.WordStore.MasteryDistribution query: %w", err)
	}
	defer rows.Close()
	out := make(map[int]int)
	for rows.Next() {
		var level int
		var count int
		if err := rows.Scan(&level, &count); err != nil {
			return nil, fmt.Errorf("data.WordStore.MasteryDistribution scan: %w", err)
		}
		out[level] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data.WordStore.MasteryDistribution rows: %w", err)
	}
	return out, nil
}
```

Extract a reusable `scanWords` helper only if it removes duplication without changing existing behavior.

- [ ] **Step 4: Add adapter forwarding methods**

In `internal/data/adapters.go`, add:

```go
func (a *WordStoreAdapter) ListReviewPlanRecords(userID int64, level word.JLPTLevel, days int) ([]word.WordReviewPlanRecord, error) {
	return a.s.ListReviewPlanRecords(userID, level, days)
}

func (a *WordStoreAdapter) ListNewWordCandidates(userID int64, level word.JLPTLevel, limit int) ([]word.Word, error) {
	return a.s.ListNewWordCandidates(userID, level, limit)
}

func (a *WordStoreAdapter) CountNewWordCandidates(userID int64, level word.JLPTLevel) (int, error) {
	return a.s.CountNewWordCandidates(userID, level)
}

func (a *WordStoreAdapter) CountTodayCompleted(userID int64) (int, error) {
	return a.s.CountTodayCompleted(userID)
}

func (a *WordStoreAdapter) MasteryDistribution(userID int64, level word.JLPTLevel) (map[int]int, error) {
	return a.s.MasteryDistribution(userID, level)
}
```

- [ ] **Step 5: Run store and module tests**

Run:

```powershell
go test ./internal/data -run TestWordStore_ReviewPlanQueries -count=1
go test ./internal/module/word -count=1
```

Expected: both PASS.

- [ ] **Step 6: Commit Task 2**

Run:

```powershell
git add -- internal/data/word_store.go internal/data/adapters.go internal/data/word_store_test.go
git commit -m "feat(word): query review plan data"
```

---

### Task 3: Review Plan HTTP Endpoint

**Files:**
- Modify: `internal/module/word/handler.go`
- Create or modify: `internal/module/word/handler_test.go`

- [ ] **Step 1: Write failing handler test**

Create `internal/module/word/handler_test.go` if it does not exist. Test invalid `days` and a successful JSON response with a fake service if the current handler is not easy to instantiate directly. Prefer testing `parseDaysParam` and `parseReviewPlanLevel` as pure helpers:

```go
func TestParseDaysParam(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{name: "default", raw: "", want: 7},
		{name: "valid", raw: "14", want: 14},
		{name: "too low defaults", raw: "0", want: 7},
		{name: "too high clamps", raw: "45", want: 30},
		{name: "invalid defaults", raw: "abc", want: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDaysParam(tt.raw); got != tt.want {
				t.Fatalf("parseDaysParam(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run handler test and verify it fails**

Run:

```powershell
go test ./internal/module/word -run TestParseDaysParam -count=1
```

Expected: FAIL because `parseDaysParam` does not exist.

- [ ] **Step 3: Add route and handler**

In `RegisterRoutes`, add the static route before `GET /api/v1/words/{id}`:

```go
	mux.HandleFunc("GET /api/v1/words/review-plan", h.handleGetReviewPlan)
```

Add:

```go
func (h *WordHandler) handleGetReviewPlan(w http.ResponseWriter, r *http.Request) {
	userID, ok := user.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "unauthorized", "")
		return
	}

	level := JLPTLevel(r.URL.Query().Get("level"))
	if level == "" {
		level = LevelN5
	}
	days := parseDaysParam(r.URL.Query().Get("days"))
	dailyGoal := 20
	if h.dailyGoal != nil {
		if goal := h.dailyGoal.GetWordDailyGoal(userID); goal >= 0 {
			dailyGoal = goal
		}
	}

	plan, err := h.svc.GetReviewPlan(userID, level, days, dailyGoal, time.Now())
	if err != nil {
		slog.Error("handleGetReviewPlan failed", "err", err, "user_id", userID, "level", level)
		httputil.WriteError(w, http.StatusInternalServerError, "ERR_INTERNAL", "failed to load review plan", "")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, httputil.APIResponse{Data: plan})
}

func parseDaysParam(raw string) int {
	if raw == "" {
		return 7
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 7
	}
	if n > 30 {
		return 30
	}
	return n
}
```

Add `time` to imports.

- [ ] **Step 4: Run handler and module tests**

Run:

```powershell
go test ./internal/module/word -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 3**

Run:

```powershell
git add -- internal/module/word/handler.go internal/module/word/handler_test.go
git commit -m "feat(word): expose review plan endpoint"
```

---

### Task 4: Frontend Types And Filter Helpers

**Files:**
- Modify: `front/react/src/types/api.ts`
- Create: `front/react/src/pages/word/reviewPlan.ts`
- Create: `front/react/src/pages/word/__tests__/reviewPlan.test.ts`

- [ ] **Step 1: Write failing frontend helper tests**

Create `front/react/src/pages/word/__tests__/reviewPlan.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { WordReviewPlanItem } from '@/types/api'
import { filterReviewPlanItems, getReviewPlanStatusLabel } from '../reviewPlan'

function item(status: WordReviewPlanItem['status']): WordReviewPlanItem {
  return {
    word: {
      id: 1,
      kanji_form: '計画',
      reading: 'けいかく',
      part_of_speech: '名詞',
      meaning: '计划',
      jlpt_level: 'N5',
      examples: [],
      reading_type: '',
    },
    status,
    days_until_review: status === 'future' ? 2 : 0,
  }
}

describe('review plan helpers', () => {
  it('filters items by status', () => {
    const items = [item('due'), item('overdue'), item('future'), item('new'), item('mastered')]

    expect(filterReviewPlanItems(items, 'all')).toHaveLength(5)
    expect(filterReviewPlanItems(items, 'today').map(i => i.status)).toEqual(['due'])
    expect(filterReviewPlanItems(items, 'overdue').map(i => i.status)).toEqual(['overdue'])
    expect(filterReviewPlanItems(items, 'future').map(i => i.status)).toEqual(['future'])
    expect(filterReviewPlanItems(items, 'new').map(i => i.status)).toEqual(['new'])
    expect(filterReviewPlanItems(items, 'mastered').map(i => i.status)).toEqual(['mastered'])
  })

  it('returns readable Chinese status labels', () => {
    expect(getReviewPlanStatusLabel('due')).toBe('今天')
    expect(getReviewPlanStatusLabel('overdue')).toBe('逾期')
    expect(getReviewPlanStatusLabel('future')).toBe('未来')
    expect(getReviewPlanStatusLabel('new')).toBe('新词')
    expect(getReviewPlanStatusLabel('mastered')).toBe('已掌握')
  })
})
```

- [ ] **Step 2: Run frontend test and verify it fails**

Run:

```powershell
cd front/react
npm test -- --run src/pages/word/__tests__/reviewPlan.test.ts
```

Expected: FAIL because the types and helper file do not exist.

- [ ] **Step 3: Add TypeScript API types**

In `front/react/src/types/api.ts`, add:

```ts
export type WordReviewPlanStatus = 'overdue' | 'due' | 'future' | 'mastered' | 'new'

export type WordReviewPlanFilter = 'all' | 'today' | 'overdue' | 'future' | 'mastered' | 'new'

export interface WordReviewPlanDay {
  date: string
  due_count: number
  overdue_count: number
  new_slots: number
  total_planned: number
}

export interface WordReviewPlanItem {
  word: Word
  record?: WordRecord
  status: WordReviewPlanStatus
  days_until_review: number
}

export interface WordReviewPlan {
  level: JLPTLevel
  daily_goal: number
  today_completed: number
  due_count: number
  new_candidates_count: number
  future_due_count: number
  days: WordReviewPlanDay[]
  mastery_distribution: Record<string, number>
  items: WordReviewPlanItem[]
}
```

- [ ] **Step 4: Add helper implementation**

Create `front/react/src/pages/word/reviewPlan.ts`:

```ts
import type { WordReviewPlanFilter, WordReviewPlanItem, WordReviewPlanStatus } from '@/types/api'

export const REVIEW_PLAN_FILTERS: Array<{ key: WordReviewPlanFilter; label: string }> = [
  { key: 'all', label: '全部' },
  { key: 'today', label: '今天' },
  { key: 'overdue', label: '逾期' },
  { key: 'future', label: '未来 7 天' },
  { key: 'mastered', label: '已掌握' },
  { key: 'new', label: '新词候选' },
]

export function filterReviewPlanItems(items: WordReviewPlanItem[], filter: WordReviewPlanFilter) {
  if (filter === 'all') return items
  if (filter === 'today') return items.filter(item => item.status === 'due')
  return items.filter(item => item.status === filter)
}

export function getReviewPlanStatusLabel(status: WordReviewPlanStatus) {
  const labels: Record<WordReviewPlanStatus, string> = {
    overdue: '逾期',
    due: '今天',
    future: '未来',
    mastered: '已掌握',
    new: '新词',
  }
  return labels[status]
}

export function formatReviewDate(date: string) {
  const parsed = new Date(`${date}T00:00:00`)
  if (Number.isNaN(parsed.getTime())) return date
  return `${parsed.getMonth() + 1}/${parsed.getDate()}`
}
```

- [ ] **Step 5: Run frontend helper tests**

Run:

```powershell
cd front/react
npm test -- --run src/pages/word/__tests__/reviewPlan.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit Task 4**

Run:

```powershell
git add -- front/react/src/types/api.ts front/react/src/pages/word/reviewPlan.ts front/react/src/pages/word/__tests__/reviewPlan.test.ts
git commit -m "feat(word): add review plan frontend helpers"
```

---

### Task 5: Frontend Review Plan Page And Routes

**Files:**
- Create: `front/react/src/pages/word/WordReviewPlanPage.tsx`
- Create: `front/react/src/pages/word/WordReviewPlanPage.module.css`
- Modify: `front/react/src/App.tsx`
- Modify: `front/react/src/pages/home/HomePage.tsx`
- Modify: `front/react/src/pages/word/WordReviewPage.tsx`
- Modify: `front/react/src/i18n/locales/zh.ts`
- Modify: `front/react/src/i18n/locales/en.ts`
- Modify: `front/react/src/i18n/locales/ja.ts`

- [ ] **Step 1: Add page route shell**

Create `front/react/src/pages/word/WordReviewPlanPage.tsx`:

```tsx
import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { EmptyState } from '@/components/ui/EmptyState'
import { Spinner } from '@/components/ui/Spinner'
import type { JLPTLevel, WordReviewPlan, WordReviewPlanFilter } from '@/types/api'
import { REVIEW_PLAN_FILTERS, filterReviewPlanItems, formatReviewDate, getReviewPlanStatusLabel } from './reviewPlan'
import styles from './WordReviewPlanPage.module.css'

const LEVELS: JLPTLevel[] = ['N5', 'N4', 'N3', 'N2', 'N1']

export function WordReviewPlanPage() {
  const [level, setLevel] = useState<JLPTLevel>('N5')
  const [filter, setFilter] = useState<WordReviewPlanFilter>('all')
  const [plan, setPlan] = useState<WordReviewPlan | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    apiFetch<WordReviewPlan>('GET', `/api/v1/words/review-plan?level=${level}&days=7`, undefined, controller.signal)
      .then(data => setPlan(data))
      .catch(err => {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Failed to load review plan')
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [level])

  const items = useMemo(() => filterReviewPlanItems(plan?.items ?? [], filter), [plan, filter])

  if (loading) {
    return <div className={styles.center}><Spinner size="lg" /></div>
  }

  if (error) {
    return <EmptyState icon="📅" title="复习计划加载失败" description={error} />
  }

  if (!plan) {
    return <EmptyState icon="📅" title="暂无复习计划" description="当前没有可展示的复习安排。" />
  }

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div>
          <p className={styles.eyebrow}>单词复习</p>
          <h1>复习计划</h1>
        </div>
        <Link className={styles.primaryButton} to="/words/review">开始复习</Link>
      </header>

      <div className={styles.tabs}>
        {LEVELS.map(lv => (
          <button key={lv} className={`${styles.tab} ${level === lv ? styles.tabActive : ''}`} onClick={() => setLevel(lv)}>
            {lv}
          </button>
        ))}
      </div>

      <section className={styles.summaryGrid}>
        <Metric label="今日待复习" value={plan.due_count} />
        <Metric label="今日目标" value={plan.daily_goal} />
        <Metric label="今日已完成" value={plan.today_completed} />
        <Metric label="未来 7 天" value={plan.future_due_count} />
      </section>

      <section className={styles.panel}>
        <div className={styles.panelHeader}>
          <h2>7 天计划</h2>
          <span>{plan.new_candidates_count} 个新词候选</span>
        </div>
        <div className={styles.daysGrid}>
          {plan.days.map(day => (
            <div key={day.date} className={styles.dayCard}>
              <strong>{formatReviewDate(day.date)}</strong>
              <span>到期 {day.due_count}</span>
              <span>逾期 {day.overdue_count}</span>
              <span>新词 {day.new_slots}</span>
              <em>{day.total_planned}</em>
            </div>
          ))}
        </div>
      </section>

      <section className={styles.panel}>
        <div className={styles.panelHeader}>
          <h2>掌握度分布</h2>
          <Badge level={level} size="sm" />
        </div>
        <div className={styles.masteryGrid}>
          {['0', '1', '2', '3', '4', '5'].map(key => (
            <div key={key}>
              <span>{key}/5</span>
              <strong>{plan.mastery_distribution[key] ?? 0}</strong>
            </div>
          ))}
        </div>
      </section>

      <section className={styles.panel}>
        <div className={styles.panelHeader}>
          <h2>单词清单</h2>
          <span>{items.length} 项</span>
        </div>
        <div className={styles.filters}>
          {REVIEW_PLAN_FILTERS.map(f => (
            <button key={f.key} className={`${styles.filterButton} ${filter === f.key ? styles.filterActive : ''}`} onClick={() => setFilter(f.key)}>
              {f.label}
            </button>
          ))}
        </div>
        {items.length === 0 ? (
          <EmptyState icon="📖" title="暂无单词" description="当前筛选条件下没有单词。" />
        ) : (
          <div className={styles.wordList}>
            {items.map(item => (
              <article key={`${item.status}-${item.word.id}`} className={styles.wordRow}>
                <div>
                  <strong>{item.word.kanji_form}</strong>
                  <span>{item.word.reading} · {item.word.meaning}</span>
                </div>
                <div className={styles.wordMeta}>
                  <Badge level={item.word.jlpt_level} size="sm" />
                  <span>{getReviewPlanStatusLabel(item.status)}</span>
                  <span>掌握 {item.record?.mastery_level ?? 0}/5</span>
                  <span>间隔 {item.record?.interval ?? 0} 天</span>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className={styles.metric}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}
```

- [ ] **Step 2: Add page styles**

Create `front/react/src/pages/word/WordReviewPlanPage.module.css` with layout classes referenced above:

```css
.page {
  width: min(calc(100vw - var(--space-8)), 1120px);
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.center {
  display: flex;
  justify-content: center;
  padding-top: 80px;
}

.header,
.panelHeader {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.eyebrow {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  margin-bottom: var(--space-1);
}

.header h1,
.panelHeader h2 {
  margin: 0;
  letter-spacing: 0;
}

.primaryButton {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 40px;
  padding: 0 var(--space-5);
  border-radius: var(--radius-sm);
  background: var(--color-brand);
  color: var(--color-text-inverse);
  font-weight: var(--font-weight-semibold);
}

.tabs,
.filters {
  display: flex;
  gap: var(--space-2);
  overflow-x: auto;
  padding-bottom: var(--space-1);
}

.tab,
.filterButton {
  min-height: 34px;
  padding: 0 var(--space-4);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-full);
  background: var(--color-bg-surface);
  color: var(--color-text-secondary);
  font-weight: var(--font-weight-semibold);
  white-space: nowrap;
}

.tabActive,
.filterActive {
  border-color: var(--color-brand);
  background: var(--color-brand-light);
  color: var(--color-brand);
}

.summaryGrid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-3);
}

.metric,
.panel,
.dayCard,
.wordRow {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg-surface);
  box-shadow: var(--shadow-xs);
}

.metric {
  padding: var(--space-4);
}

.metric span,
.panelHeader span,
.wordRow span {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.metric strong {
  display: block;
  margin-top: var(--space-2);
  font-size: var(--font-size-2xl);
}

.panel {
  padding: var(--space-5);
}

.daysGrid {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: var(--space-2);
}

.dayCard {
  display: grid;
  gap: var(--space-1);
  padding: var(--space-3);
}

.dayCard em {
  margin-top: var(--space-1);
  color: var(--color-brand);
  font-style: normal;
  font-weight: var(--font-weight-bold);
}

.masteryGrid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: var(--space-2);
}

.masteryGrid div {
  padding: var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--color-bg);
}

.masteryGrid strong {
  display: block;
  margin-top: var(--space-1);
}

.wordList {
  display: grid;
  gap: var(--space-2);
}

.wordRow {
  display: flex;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-3);
}

.wordRow strong {
  display: block;
}

.wordMeta {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
  flex-wrap: wrap;
}

@media (max-width: 860px) {
  .summaryGrid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .daysGrid,
  .masteryGrid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .wordRow {
    flex-direction: column;
  }

  .wordMeta {
    justify-content: flex-start;
  }
}
```

- [ ] **Step 3: Wire the route**

In `front/react/src/App.tsx`, import and route the page:

```tsx
import { WordReviewPlanPage } from './pages/word/WordReviewPlanPage'
```

Add inside protected routes:

```tsx
<Route path="/words/review-plan" element={<WordReviewPlanPage />} />
```

- [ ] **Step 4: Update entry links**

In `front/react/src/pages/home/HomePage.tsx`, change:

```tsx
<Link to="/words/review" className={styles.tipLink}>{t('home.viewReviewPlan')} →</Link>
```

to:

```tsx
<Link to="/words/review-plan" className={styles.tipLink}>{t('home.viewReviewPlan')} →</Link>
```

In `front/react/src/pages/word/WordReviewPage.tsx`, import `Link` from `react-router-dom` and add inside `.header`:

```tsx
<Link to="/words/review-plan" className={styles.planLink}>查看计划</Link>
```

Add `.planLink` to `WordReviewPage.module.css`:

```css
.planLink {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 34px;
  padding: 0 var(--space-4);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-bg-surface);
  color: var(--color-text);
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
}
```

- [ ] **Step 5: Add locale keys**

Add namespaced keys under `word.reviewPlan` in each locale file. Chinese:

```ts
reviewPlan: {
  title: '复习计划',
  eyebrow: '单词复习',
  startReview: '开始复习',
  loadError: '复习计划加载失败',
  emptyTitle: '暂无复习计划',
  emptyDesc: '当前没有可展示的复习安排。',
  todayDue: '今日待复习',
  dailyGoal: '今日目标',
  todayCompleted: '今日已完成',
  futureSevenDays: '未来 7 天',
  sevenDayPlan: '7 天计划',
  newCandidates: '{{count}} 个新词候选',
  masteryDistribution: '掌握度分布',
  wordList: '单词清单',
  itemsCount: '{{count}} 项',
  noWords: '暂无单词',
  noWordsDesc: '当前筛选条件下没有单词。',
  due: '到期 {{count}}',
  overdue: '逾期 {{count}}',
  newWords: '新词 {{count}}',
  mastery: '掌握 {{level}}/5',
  interval: '间隔 {{count}} 天',
  viewPlan: '查看计划',
}
```

Update the page to use `useTranslation()` and these keys instead of hard-coded Chinese strings.

- [ ] **Step 6: Run frontend tests and build**

Run:

```powershell
cd front/react
npm test
npm run build
```

Expected: PASS. Existing Vite browser-compatibility warnings from `kuromojin` may remain.

- [ ] **Step 7: Commit Task 5**

Run:

```powershell
git add -- front/react/src/pages/word/WordReviewPlanPage.tsx front/react/src/pages/word/WordReviewPlanPage.module.css front/react/src/App.tsx front/react/src/pages/home/HomePage.tsx front/react/src/pages/word/WordReviewPage.tsx front/react/src/pages/word/WordReviewPage.module.css front/react/src/i18n/locales/zh.ts front/react/src/i18n/locales/en.ts front/react/src/i18n/locales/ja.ts
git commit -m "feat(word): add review plan page"
```

---

### Task 6: End-To-End Verification

**Files:**
- No source files unless verification exposes a bug.

- [ ] **Step 1: Run full backend tests**

Run:

```powershell
make test
```

Expected: PASS.

- [ ] **Step 2: Run frontend verification**

Run:

```powershell
cd front/react
npm test
npm run build
```

Expected: PASS.

- [ ] **Step 3: Verify in the in-app browser**

Use the Browser plugin on:

```text
http://localhost:5173/words/review-plan
```

Required checks:

- Page identity is `/words/review-plan`.
- Page is not blank and shows the review plan title.
- No framework error overlay appears.
- Console has no new application errors.
- Switching JLPT tabs refreshes visible page state.
- Clicking a filter changes the visible item count or empty state.
- Clicking “开始复习” navigates to `/words/review`.

- [ ] **Step 4: Final diff review**

Run:

```powershell
git diff --check
git status --short
```

Expected: `git diff --check` exits 0. `git status --short` may still show unrelated pre-existing workspace changes; report only files touched by this feature.

- [ ] **Step 5: Completion commit if verification fixes were needed**

If Task 6 required source fixes in frontend route wiring, commit them:

```powershell
git add -- front/react/src/App.tsx front/react/src/pages/word/WordReviewPlanPage.tsx front/react/src/pages/word/WordReviewPlanPage.module.css front/react/src/pages/home/HomePage.tsx front/react/src/pages/word/WordReviewPage.tsx front/react/src/pages/word/WordReviewPage.module.css front/react/src/i18n/locales/zh.ts front/react/src/i18n/locales/en.ts front/react/src/i18n/locales/ja.ts
git commit -m "fix(word): polish review plan verification issues"
```

If Task 6 required no source fixes, do not create an empty commit.

---

## Self-Review Notes

- Spec coverage: backend API, plan math, frontend route, homepage entry, review-page entry, filters, tests, build, and browser verification are each represented by tasks.
- Type consistency: Go names use `ReviewPlan`, `ReviewPlanDay`, `ReviewPlanItem`, `ReviewPlanStatus`; TypeScript names use `WordReviewPlan`, `WordReviewPlanDay`, `WordReviewPlanItem`, `WordReviewPlanStatus`.
- Scope: only single-word review plan is included; cross-module planning and manual date editing are excluded.
