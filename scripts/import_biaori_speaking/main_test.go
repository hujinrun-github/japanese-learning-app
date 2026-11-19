package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/testdb"
)

func TestCanonical(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"1. 今4時です。\n甲：何時ですか。", "今4時です何時ですか"},
		{"A: こんにちは！\nB:はい。", "こんにちははい"},
		{"小野：こんにちは。", "こんにちは"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			if got := canonical(tc.text); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestImportPackPostgres(t *testing.T) {
	for _, apply := range []bool{false, true} {
		t.Run(fmt.Sprintf("apply=%v", apply), func(t *testing.T) {
			db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
			items := []material{sample("first", "甲：こんにちは。"), sample("duplicate", "こんにちは！"), sample("second", "さようなら。")}
			result, err := importPack(context.Background(), db, items, apply)
			if err != nil {
				t.Fatal(err)
			}
			if result.Pending != 2 || len(result.Skipped) != 1 {
				t.Fatalf("unexpected result: %+v", result)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM speaking_materials").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if apply && count != 2 || !apply && count != 0 {
				t.Fatalf("material count=%d", count)
			}
			if !apply {
				return
			}
			var linked int
			if err := db.QueryRow(`SELECT count(*) FROM speaking_materials sm JOIN audio_objects ao ON sm.audio_object_id=ao.id WHERE sm.audio_url_legacy='/api/v1/audio/' || ao.id || '/stream' AND cardinality(sm.lines)>0`).Scan(&linked); err != nil {
				t.Fatal(err)
			}
			if linked != 2 {
				t.Fatalf("linked=%d", linked)
			}
			again, err := importPack(context.Background(), db, items, true)
			if err != nil {
				t.Fatal(err)
			}
			if again.Pending != 0 || len(again.Skipped) != 3 {
				t.Fatalf("repeat: %+v", again)
			}
		})
	}
}

func TestAudioConflictRollsBackWholePack(t *testing.T) {
	db := testdb.Open(t, (postgres.Adapter{}).RunMigrations)
	items := []material{sample("first", "こんにちは。"), sample("second", "さようなら。")}
	if _, err := db.Exec(`INSERT INTO audio_objects (bucket,object_key,kind,visibility,content_sha256,size_bytes,mime_type) VALUES ($1,$2,'example_audio','public','different',1,'audio/wav')`, items[1].Bucket, items[1].ObjectKey); err != nil {
		t.Fatal(err)
	}
	if _, err := importPack(context.Background(), db, items, true); err == nil {
		t.Fatal("expected audio checksum conflict")
	}
	var materials, audios int
	if err := db.QueryRow("SELECT (SELECT count(*) FROM speaking_materials), (SELECT count(*) FROM audio_objects)").Scan(&materials, &audios); err != nil {
		t.Fatal(err)
	}
	if materials != 0 || audios != 1 {
		t.Fatalf("partial writes: materials=%d audios=%d", materials, audios)
	}
}

func sample(title, text string) material {
	return material{Type: "shadow", Title: title, Text: text, Level: "N5", Lines: []string{text}, Bucket: "japanese", ObjectKey: fmt.Sprintf("examples/%x.wav", sha256.Sum256([]byte(text)))[:25] + ".wav", ContentSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("audio"))), SizeBytes: 500, Metadata: []byte(`{"tts_provider":"test"}`)}
}
