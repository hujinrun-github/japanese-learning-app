package translation_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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

func TestTranslationDailyPracticeMatchesSavedLevels(t *testing.T) {
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	st := pgdata.NewTranslationStore(db, store.StoreDeps{Clock: translationClock{now}, AppTimezone: time.UTC})
	if _, err := db.Exec(`INSERT INTO users (id,email,password_hash) VALUES (1,'levels@example.test','unused');
		INSERT INTO translation_sources(id,title,source_type,raw_content)
		VALUES (1,'标题里的 N4 并非分级依据','manual',''), (2,'手动导入','manual','');
		INSERT INTO translation_sentences (id,source_id,direction,source_text,position,jlpt_level)
		VALUES (10,1,'jp2cn','N4 practice',0,'N4'), (11,1,'jp2cn','N2 practice',1,'N2'),
		       (12,1,'jp2cn','N2 new',2,'N2'), (13,1,'jp2cn','N3 new',3,'N3'),
		       (14,2,'jp2cn','Ungraded',0,'')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{10, 11, 14} {
		if _, err := st.SaveRecord(translation.TranslationRecord{
			UserID: 1, SentenceID: id, UserTranslation: "回答", PracticedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	const secret = "translation-level-test"
	mux := http.NewServeMux()
	translation.NewTranslationHandler(translation.NewTranslationService(st, nil)).RegisterRoutes(mux)
	handler := user.AuthMiddleware(secret, mux)
	token, _, err := user.SignToken(1, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string, out any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, body = %s", path, resp.Code, resp.Body)
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &struct{ Data any }{Data: out}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, storedLevels string
		levels             []string
		done               int
		ids                []int64
	}{
		{"one level", `["N2"]`, []string{"N2"}, 1, []int64{12}},
		{"multiple levels", `["N2","N3"]`, []string{"N2", "N3"}, 1, []int64{12, 13}},
		{"changing settings scopes existing progress", `["N3"]`, []string{"N3"}, 0, []int64{13}},
		{"no questions for the selected level", `["N1"]`, []string{"N1"}, 0, nil},
		{"old profile without a saved level", `[]`, []string{"N5"}, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.Exec(`UPDATE users SET jlpt_levels=ARRAY(SELECT jsonb_array_elements_text($1::jsonb)) WHERE id=1`, tc.storedLevels); err != nil {
				t.Fatal(err)
			}
			var progress struct {
				Levels []string `json:"jlpt_levels"`
				Done   int      `json:"today_completed"`
			}
			get("/api/v1/translation/progress", &progress)
			if progress.Done != tc.done || !slices.Equal(progress.Levels, tc.levels) {
				t.Errorf("progress = %+v, want %d completed at %v", progress, tc.done, tc.levels)
			}
			for _, suffix := range []string{"", "?count=20"} {
				var queued []translation.TranslationSentence
				get("/api/v1/translation/queue"+suffix, &queued)
				var got []int64
				for _, item := range queued {
					got = append(got, item.ID)
				}
				slices.Sort(got)
				if !slices.Equal(got, tc.ids) {
					t.Errorf("queue%s = %v, want %v", suffix, got, tc.ids)
				}
			}
		})
	}
	var sources []struct {
		ID     int64
		Levels []string `json:"jlpt_levels"`
	}
	get("/api/v1/translation/sources", &sources)
	for _, source := range sources {
		if source.ID == 1 && !slices.Equal(source.Levels, []string{"N4", "N3", "N2"}) {
			t.Errorf("mixed source labels = %v, want N4/N3/N2", source.Levels)
		}
		if source.ID == 2 && len(source.Levels) != 0 {
			t.Errorf("ungraded source labels = %v", source.Levels)
		}
	}
	var ungraded []translation.TranslationSentence
	get("/api/v1/translation/sentences?source_id=2", &ungraded)
	if len(ungraded) != 1 || ungraded[0].ID != 14 {
		t.Fatalf("ungraded free practice = %+v", ungraded)
	}
}

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
			SourceID: sourceID, Direction: "jp2cn", SourceText: fmt.Sprintf("問題%dです。", i), Position: i, JLPTLevel: "N5",
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}

	const secret = "translation-daily-test"
	const queue = "/api/v1/translation/queue"
	mux := http.NewServeMux()
	translation.NewTranslationHandler(translation.NewTranslationService(store, studyplan.ResolveTranslationTarget)).RegisterRoutes(mux)
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
