package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pgschema "japanese-learning-app/internal/data/migrations/postgres"
	"japanese-learning-app/internal/testdb"
)

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
