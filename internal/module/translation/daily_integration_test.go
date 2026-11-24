package translation_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/module/studyplan"
	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/store"
	"japanese-learning-app/internal/testdb"
)

type translationClock struct{ time.Time }

func (c translationClock) Now() time.Time { return c.Time }

func TestTranslationDailyProgressAndQueueUseTheScheduledGoal(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	// UTC is still Monday, but the learner's plan is already on Tuesday.
	now := time.Date(2026, 11, 23, 17, 0, 0, 0, time.UTC)
	store := pgdata.NewTranslationStore(db, store.StoreDeps{Clock: translationClock{now}, AppTimezone: time.UTC})
	sourceID, err := store.SaveSource(translation.TranslationSource{Title: "Daily practice", SourceType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i := range 35 {
		id, err := store.SaveSentence(translation.TranslationSentence{
			SourceID: sourceID, Direction: "jp2cn", SourceText: fmt.Sprintf("問題%dです。", i), Position: i,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}

	const secret = "translation-daily-test"
	const queue = "/api/v1/translation/queue"
	mux := http.NewServeMux()
	translation.NewTranslationHandler(translation.NewTranslationService(store, nil, studyplan.ResolveTranslationTarget)).RegisterRoutes(mux)
	handler := user.AuthMiddleware(secret, mux)

	for i, tc := range []struct {
		name                string
		title               string
		path                string
		taskDays            []int
		legacyGoal          int
		settingsGoal        int
		done                int
		includePriorPlanDay bool
		wantGoal            int
		wantDone            int
	}{
		{name: "scheduled ten", title: "翻译10题", path: "/translation", settingsGoal: 5, wantGoal: 10},
		{name: "resume on plan's local day", title: "翻译 １０ 题", path: "/translation", settingsGoal: 5, done: 3, includePriorPlanDay: true, wantGoal: 10, wantDone: 3},
		{name: "completed goal", title: "翻译10题", path: "/translation", settingsGoal: 5, done: 10, wantGoal: 10, wantDone: 10},
		{name: "large configured round", title: "翻译30题", path: "/translation", settingsGoal: 5, wantGoal: 30},
		{name: "task on a different weekday", title: "翻译10题", path: "/translation", taskDays: []int{1}, settingsGoal: 7, wantGoal: 7},
		{name: "other module with the same title", title: "翻译10题", path: "/grammar", settingsGoal: 7, wantGoal: 7},
		{name: "legacy translation goal", legacyGoal: 8, settingsGoal: 5, wantGoal: 8},
		{name: "settings fallback", settingsGoal: 7, wantGoal: 7},
		{name: "disabled goal in settings", settingsGoal: 0, wantGoal: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uid := int64(i + 1)
			_, err := db.Exec(`INSERT INTO users (id, email, password_hash, daily_goals_json)
				VALUES ($1, $2, 'unused', jsonb_build_object('translation', $3::int))`, uid, fmt.Sprintf("learner%d@example.com", uid), tc.settingsGoal)
			if err != nil {
				t.Fatal(err)
			}
			if tc.title != "" || tc.legacyGoal > 0 {
				plan := studyplan.Plan{Title: "学习计划", StartDate: "2026-11-20", EndDate: "2026-11-30", Timezone: "Asia/Shanghai",
					Weekdays: []int{1, 2}, Time: "20:00", Duration: 30, Translation: tc.legacyGoal}
				if tc.title != "" {
					plan.TimeSlots = []studyplan.TimeSlot{{ID: "night", Title: "晚间学习", Time: "20:00", Duration: 30,
						Tasks: []studyplan.Task{{ID: "practice", Title: tc.title, Path: tc.path, Minutes: 5, Weekdays: tc.taskDays}}}}
				}
				raw, err := json.Marshal(plan)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO study_plans (user_id, plan_json, calendar_token) VALUES ($1, $2, $3)`, uid, string(raw), fmt.Sprint(uid)); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range ids[:tc.done] {
				if _, err := store.SaveRecord(translation.TranslationRecord{
					UserID: uid, SentenceID: id, UserTranslation: "回答", PracticedAt: now.Add(-30 * time.Minute),
				}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.includePriorPlanDay {
				if _, err := store.SaveRecord(translation.TranslationRecord{
					UserID: uid, SentenceID: ids[tc.done], UserTranslation: "回答", PracticedAt: now.Add(-90 * time.Minute),
				}); err != nil {
					t.Fatal(err)
				}
			}
			token, _, err := user.SignToken(uid, secret, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			get := func(path string, value any) {
				t.Helper()
				req := httptest.NewRequest(http.MethodGet, path, nil)
				req.Header.Set("Authorization", "Bearer "+token)
				resp := httptest.NewRecorder()
				handler.ServeHTTP(resp, req)
				if resp.Code != http.StatusOK {
					t.Fatalf("GET %s: status = %d, body = %s", path, resp.Code, resp.Body)
				}
				if err := json.Unmarshal(resp.Body.Bytes(), &struct {
					Data any `json:"data"`
				}{Data: value}); err != nil {
					t.Fatal(err)
				}
			}
			var progress struct {
				Goal      int `json:"daily_goal"`
				Completed int `json:"today_completed"`
			}
			get("/api/v1/translation/progress", &progress)
			if progress.Goal != tc.wantGoal || progress.Completed != tc.wantDone {
				t.Errorf("progress = %d/%d, want %d/%d", progress.Completed, progress.Goal, tc.wantDone, tc.wantGoal)
			}
			var questions []translation.TranslationSentence
			get(queue, &questions)
			if want := tc.wantGoal - tc.wantDone; len(questions) != want {
				t.Errorf("queue length = %d, want %d", len(questions), want)
			}
			get(queue+"?count=2", &questions)
			if len(questions) != 2 {
				t.Errorf("explicit queue length = %d, want 2", len(questions))
			}
		})
	}
}
