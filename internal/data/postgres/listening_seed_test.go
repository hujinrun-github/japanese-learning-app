package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pgschema "japanese-learning-app/internal/data/migrations/postgres"
	"japanese-learning-app/internal/module/listening"
	"japanese-learning-app/internal/testdb"
)

func TestSeedListeningQuestionsForMyDay(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name             string
		changeSource     bool
		existingQuestion bool
		wantInserted     int64
	}{
		{name: "existing intensive progress", wantInserted: 3},
		{name: "same title with different text", changeSource: true},
		{name: "existing question stays intact", existingQuestion: true, wantInserted: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testdb.Open(t, (Adapter{}).RunMigrations)
			store := NewListeningStore(db)
			if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'my-day@example.test','test')`); err != nil {
				t.Fatal(err)
			}
			var id int64
			if err := db.QueryRow(`INSERT INTO lessons(title,jlpt_level,shadowing_config_json)
				VALUES('私の一日','N5','{"audio_url":"/audio/lessons/shadowing-pilot-n5-day.wav"}') RETURNING id`).Scan(&id); err != nil {
				t.Fatal(err)
			}
			sentences := []string{
				"毎朝、七時に起きます。",
				"朝ごはんはパンとコーヒーです。",
				"八時半に学校へ行きます。",
				"午後は図書館で勉強します。",
				"夜は家族と一緒に晩ごはんを食べます。",
				"十一時ごろに寝ます。",
			}
			if tc.changeSource {
				sentences[0] = "毎朝、八時に起きます。"
			}
			for i, sentence := range sentences {
				if _, err := db.Exec(`INSERT INTO lesson_sentences(lesson_id,position,tokens,start_ms,end_ms)
					VALUES($1,$2,jsonb_build_array(jsonb_build_object('surface',$3::text)),$4,$5)`,
					id, i, sentence, i*3000, (i+1)*3000); err != nil {
					t.Fatal(err)
				}
			}
			_, err := store.SaveProgress(ctx, 1, id, listening.ProgressRequest{
				Stage: "intensive", ExpectedCycle: 1, SentenceIndex: 2, PositionMS: 6000, Bookmarks: []int{2},
			})
			if err != nil {
				t.Fatal(err)
			}
			if tc.existingQuestion {
				if _, err := db.Exec(`INSERT INTO listening_questions(id,lesson_id,position,prompt,options_json,correct_option_index,explanation)
					VALUES('existing-my-day-question',$1,0,'作者原题','["一","二"]',0,'保留原解析')`, id); err != nil {
					t.Fatal(err)
				}
			}
			for i, want := range []int64{tc.wantInserted, 0} {
				inserted, err := SeedListeningQuestions(ctx, db)
				if err != nil {
					t.Fatal(err)
				}
				if inserted != want {
					t.Fatalf("seed #%d inserted %d, want %d", i+1, inserted, want)
				}
			}
			session, err := store.GetSession(ctx, 1, id)
			if err != nil {
				t.Fatal(err)
			}
			if tc.changeSource {
				if session.TrainingReady || len(session.Questions) != 0 {
					t.Fatalf("questions seeded against changed source: %+v", session.Questions)
				}
				return
			}
			if !session.TrainingReady || len(session.Questions) != 3 || len(session.Attempts) != 0 {
				t.Fatalf("training ready = %t, questions = %d, attempts = %d", session.TrainingReady, len(session.Questions), len(session.Attempts))
			}
			if tc.existingQuestion && session.Questions[0].Prompt != "作者原题" {
				t.Fatal("existing question was overwritten")
			}
			progress := session.Progress
			if progress.Stage != "intensive" || progress.Cycle != 1 || progress.SentenceIndex != 2 || progress.PositionMS != 6000 ||
				len(progress.Bookmarks) != 1 || progress.Bookmarks[0] != 2 {
				t.Fatalf("saved progress changed: %+v", progress)
			}
		})
	}
}

func TestSeedListeningQuestionsExactMatchAndIdempotency(t *testing.T) {
	db := testdb.Open(t, (Adapter{}).RunMigrations)
	content, err := pgschema.SchemaFS.ReadFile("schema/019_listening_content.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(content), "$listening_pack$")
	if len(parts) != 3 {
		t.Fatal("missing source-checked listening pack")
	}
	var pack []struct {
		Title     string   `json:"title"`
		Sentences []string `json:"sentences"`
		Questions []struct {
			Prompt      string   `json:"prompt"`
			Options     []string `json:"options"`
			Correct     int      `json:"correct"`
			Explanation string   `json:"explanation"`
			Evidence    int      `json:"evidence"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(parts[1]), &pack); err != nil {
		t.Fatal(err)
	}
	if len(pack) != 20 {
		t.Fatalf("materials=%d, want20", len(pack))
	}
	var firstID int64
	for i, p := range pack {
		if len(p.Questions) < 2 || len(p.Questions) > 3 {
			t.Fatalf("%s: question count %d", p.Title, len(p.Questions))
		}
		var id int64
		if err := db.QueryRow(`INSERT INTO lessons(title,jlpt_level) VALUES($1,'N2') RETURNING id`, p.Title).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstID = id
		}
		for j, s := range p.Sentences {
			tokens, err := json.Marshal([]map[string]string{{"surface": s, "reading": ""}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO lesson_sentences(lesson_id,position,tokens,chinese,start_ms,end_ms) VALUES($1,$2,$3,'原文中文',$4,$5)`, id, j, string(tokens), j*1000, (j+1)*1000); err != nil {
				t.Fatal(err)
			}
		}
		for _, q := range p.Questions {
			if q.Prompt == "" || q.Explanation == "" || len(q.Options) != 4 || q.Correct < 0 || q.Correct >= len(q.Options) || q.Evidence < 0 || q.Evidence >= len(p.Sentences) {
				t.Fatalf("invalid question: %+v", q)
			}
			seen := map[string]bool{}
			for _, option := range q.Options {
				if seen[option] || strings.TrimSpace(option) == "" {
					t.Fatalf("duplicate/empty option: %s", p.Title)
				}
				seen[option] = true
			}
		}
	}
	for i, want := range []int64{40, 0} {
		got, err := SeedListeningQuestions(context.Background(), db)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("seed #%d inserted%d, want%d", i, got, want)
		}
	}
	if _, err := db.Exec(`DELETE FROM listening_questions`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE lesson_sentences SET tokens='[{"surface":"教材已经修改","reading":""}]'::jsonb WHERE lesson_id=$1 AND position=0`, firstID); err != nil {
		t.Fatal(err)
	}
	got, err := SeedListeningQuestions(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if got != 38 {
		t.Fatalf("changed source inserted%d, want38", got)
	}
}
