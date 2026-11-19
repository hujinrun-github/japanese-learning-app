package postgres

import (
	"context"
	"database/sql"
	"path"
	"testing"

	pgschema "japanese-learning-app/internal/data/migrations/postgres"
)

func TestWritingTopicProgressBackfillsLegacyRecords(t *testing.T) {
	db := openPostgresTestDB(t)
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureSchemaMigrationsTable(ctx, conn); err != nil {
		t.Fatal(err)
	}
	names, err := embeddedMigrationNames()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name >= "021_" {
			continue
		}
		content, err := pgschema.SchemaFS.ReadFile(path.Join("schema", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := applyMigrationFile(ctx, conn, name, canonicalMigrationChecksum(content), string(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id,email,password_hash) VALUES (1,'legacy-writer@example.com','unused')`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ mode, question string }{
		{"essay", "title || E'\\n' || essay_prompt"},
		{"sentence", "title || E'\\n' || sentence_prompt"},
		{"essay", "title || E'\\n' || sentence_prompt"},
		{"sentence", "'unrelated task'"},
	} {
		if _, err := db.Exec(`INSERT INTO writing_records (user_id,type,question,user_answer) SELECT 1,$1,`+tc.question+`,'回答' FROM writing_topics WHERE id='weekend'`, tc.mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := (Adapter{}).RunMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT topic_id FROM writing_records ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []string{"weekend", "weekend", "", ""}
	var index int
	for rows.Next() {
		var topicID sql.NullString
		if err := rows.Scan(&topicID); err != nil {
			t.Fatal(err)
		}
		if index >= len(want) {
			t.Fatalf("unexpected extra record with topic_id=%+v", topicID)
		}
		if topicID.String != want[index] || topicID.Valid != (want[index] != "") {
			t.Fatalf("record %d topic_id=%+v want=%q", index, topicID, want[index])
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if index != len(want) {
		t.Fatalf("records=%d want=%d", index, len(want))
	}
}
