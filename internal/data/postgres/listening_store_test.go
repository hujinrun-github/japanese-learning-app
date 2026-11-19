package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"japanese-learning-app/internal/module/listening"
)

func listeningFixture(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`INSERT INTO lessons(title,jlpt_level,char_count,shadowing_config_json) VALUES($1,'N2',8,'{"audio_url":"/audio/fixture.wav"}') RETURNING id`, title).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lesson_sentences(lesson_id,position,tokens,chinese,start_ms,end_ms) VALUES($1,0,'[{"surface":"明日は休みです。"}]','明天休息。',0,2000)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO listening_questions(id,lesson_id,prompt,options_json,correct_option_index,explanation) VALUES($1,$2,'明天怎么样？','["休息","上班"]',0,'休み表示休息。')`, fmt.Sprintf("q-%d", id), id); err != nil {
		t.Fatal(err)
	}
	return id
}

func listeningAnswer(id int64, option int) []listening.Answer {
	return []listening.Answer{{QuestionID: fmt.Sprintf("q-%d", id), OptionIndex: &option}}
}

func TestListeningReviewIntervalsAndEarlyPractice(t *testing.T) {
	t.Setenv("REQUIRE_TEST_DATABASE", "1")
	db := newMigratedPostgresTestDB(t)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'listening@test.local','test')`); err != nil {
		t.Fatal(err)
	}
	id := listeningFixture(t, db, "Review interval fixture")
	s := NewListeningStore(db)
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	ctx := context.Background()
	round := 0
	completeRound := func(correct bool) listening.Progress {
		t.Helper()
		round++
		p, err := s.GetSession(ctx, 1, id)
		if err != nil {
			t.Fatal(err)
		}
		if p.Progress.Stage == "completed" {
			oldCheckpoint := listening.ProgressRequest{Stage: "blind", ExpectedCycle: p.Progress.Cycle}
			_, err := s.SaveProgress(ctx, 1, id, oldCheckpoint)
			var typed *listening.Error
			if !errors.As(err, &typed) || typed.Status != 409 {
				t.Fatalf("old checkpoint silently restarted completed training: %v", err)
			}
			if _, err := s.SaveProgress(ctx, 1, id, listening.ProgressRequest{Stage: "blind", ExpectedCycle: p.Progress.Cycle, Restart: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.SaveProgress(ctx, 1, id, oldCheckpoint); !errors.As(err, &typed) || typed.Status != 409 {
				t.Fatalf("old cycle overwrote the new round: %v", err)
			}
			p.Progress.Cycle++
		}
		if _, err := s.SubmitAttempt(ctx, 1, id, listening.AttemptRequest{Phase: "blind", Answers: listeningAnswer(id, 0), IdempotencyKey: fmt.Sprintf("b-%d", round)}); err != nil {
			t.Fatal(err)
		}
		for _, stage := range []string{"intensive", "retest"} {
			if _, err := s.SaveProgress(ctx, 1, id, listening.ProgressRequest{Stage: stage, ExpectedCycle: p.Progress.Cycle}); err != nil {
				t.Fatal(err)
			}
		}
		option := 0
		if !correct {
			option = 1
		}
		if _, err := s.SubmitAttempt(ctx, 1, id, listening.AttemptRequest{Phase: "retest", Answers: listeningAnswer(id, option), IdempotencyKey: fmt.Sprintf("r-%d", round)}); err != nil {
			t.Fatal(err)
		}
		result, err := s.Complete(ctx, 1, id)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := completeRound(true)
	if first.ReviewStep != 1 || first.DueAt == nil || !first.DueAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("first=%+v", first)
	}
	early := completeRound(true)
	if early.ReviewStep != 1 || !early.DueAt.Equal(*first.DueAt) {
		t.Fatalf("early practice inflated interval: %+v", early)
	}
	for _, tc := range []struct {
		name       string
		correct    bool
		step, days int
	}{{"second review", true, 2, 3}, {"third review", true, 3, 7}, {"wrong review", false, 0, 1}, {"relearn", true, 1, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			current, err := s.GetSession(ctx, 1, id)
			if err != nil {
				t.Fatal(err)
			}
			now = *current.Progress.DueAt
			materials, err := s.ListMaterials(ctx, 1, "N2", "due")
			if err != nil {
				t.Fatal(err)
			}
			if materials.Stats.Due != 1 || len(materials.Materials) != 1 {
				t.Fatalf("due=%+v", materials)
			}
			got := completeRound(tc.correct)
			if got.ReviewStep != tc.step || got.DueAt == nil || !got.DueAt.Equal(now.Add(time.Duration(tc.days)*24*time.Hour)) {
				t.Fatalf("got=%+v wantstep=%d days=%d", got, tc.step, tc.days)
			}
		})
	}
}

func TestListeningConcurrentSubmissionAndDraft(t *testing.T) {
	t.Setenv("REQUIRE_TEST_DATABASE", "1")
	db := newMigratedPostgresTestDB(t)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'race@test.local','test')`); err != nil {
		t.Fatal(err)
	}
	id := listeningFixture(t, db, "Concurrent fixture")
	s := NewListeningStore(db)
	ctx := context.Background()
	draft := &listening.DraftAnswers{Phase: "blind", Answers: listeningAnswer(id, 1)}
	assisted := true
	draft.Assisted = &assisted
	if _, err := s.SaveProgress(ctx, 1, id, listening.ProgressRequest{Stage: "blind", ExpectedCycle: 1, PositionMS: 650, Bookmarks: []int{0}, DraftAnswers: draft}); err != nil {
		t.Fatal(err)
	}
	session, err := s.GetSession(ctx, 1, id)
	if err != nil {
		t.Fatal(err)
	}
	if session.Progress.DraftAnswers == nil || *session.Progress.DraftAnswers.Answers[0].OptionIndex != 1 {
		t.Fatal("draft not restored")
	}
	if _, err := s.SaveProgress(ctx, 1, id, listening.ProgressRequest{Stage: "blind", ExpectedCycle: 1, PositionMS: 850}); err != nil {
		t.Fatal(err)
	}
	req := listening.AttemptRequest{Phase: "blind", Answers: listeningAnswer(id, 1), IdempotencyKey: "concurrent-key"}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			a, err := s.SubmitAttempt(ctx, 1, id, req)
			if err == nil && (a.CorrectCount != 0 || a.ID == 0 || !a.Assisted) {
				err = fmt.Errorf("invalid concurrent result: %+v", a)
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM listening_attempts WHERE lesson_id=$1`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("stored %d attempts", count)
	}
	session, err = s.GetSession(ctx, 1, id)
	if err != nil {
		t.Fatal(err)
	}
	if session.Progress.DraftAnswers != nil || len(session.Attempts) != 1 {
		t.Fatalf("submitted state=%+v", session)
	}
}

func TestListeningMaterialReadinessAndValidation(t *testing.T) {
	t.Setenv("REQUIRE_TEST_DATABASE", "1")
	db := newMigratedPostgresTestDB(t)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'validate@test.local','test')`); err != nil {
		t.Fatal(err)
	}
	s := NewListeningStore(db)
	ctx := context.Background()
	for _, tc := range []struct {
		name, mutation string
		wantStatus     int
	}{{"no audio", `UPDATE lessons SET shadowing_config_json='{}' WHERE id=$1`, 404}, {"empty timestamps", `UPDATE lesson_sentences SET end_ms=0 WHERE lesson_id=$1`, 422}, {"no questions", `DELETE FROM listening_questions WHERE lesson_id=$1`, 422}} {
		t.Run(tc.name, func(t *testing.T) {
			id := listeningFixture(t, db, tc.name)
			if _, err := db.Exec(tc.mutation, id); err != nil {
				t.Fatal(err)
			}
			_, err := s.SubmitAttempt(ctx, 1, id, listening.AttemptRequest{Phase: "blind", Answers: listeningAnswer(id, 0), IdempotencyKey: tc.name})
			var typed *listening.Error
			if !errors.As(err, &typed) || typed.Status != tc.wantStatus {
				t.Fatalf("error=%v wantstatus=%d", err, tc.wantStatus)
			}
			if tc.name != "no audio" {
				if _, err := s.SaveProgress(ctx, 1, id, listening.ProgressRequest{Stage: "intensive", ExpectedCycle: 1}); err != nil {
					t.Fatalf("unready material should allow intensive-only: %v", err)
				}
				if _, err := s.Complete(ctx, 1, id); err == nil {
					t.Fatal("unready material completed")
				}
			}
		})
	}
	id := listeningFixture(t, db, "Answer validation")
	for _, tc := range []struct {
		name    string
		answers []listening.Answer
	}{{"missing", nil}, {"unknown", []listening.Answer{{QuestionID: "missing", OptionIndex: new(int)}}}, {"missing option", []listening.Answer{{QuestionID: fmt.Sprintf("q-%d", id)}}}, {"out of range", listeningAnswer(id, 8)}, {"duplicates", append(listeningAnswer(id, 0), listeningAnswer(id, 0)...)}} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.SubmitAttempt(ctx, 1, id, listening.AttemptRequest{Phase: "blind", Answers: tc.answers, IdempotencyKey: tc.name})
			var typed *listening.Error
			if !errors.As(err, &typed) || typed.Status != 422 {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestListeningCatalogRejectsOverlappingTimestamps(t *testing.T) {
	t.Setenv("REQUIRE_TEST_DATABASE", "1")
	db := newMigratedPostgresTestDB(t)
	id := listeningFixture(t, db, "Overlapping timeline")
	if _, err := db.Exec(`INSERT INTO lesson_sentences(lesson_id,position,tokens,chinese,start_ms,end_ms) VALUES($1,1,'[{"surface":"次の文。"}]','下一句。',1000,3000)`, id); err != nil {
		t.Fatal(err)
	}
	s := NewListeningStore(db)
	session, err := s.GetSession(context.Background(), 1, id)
	if err != nil {
		t.Fatal(err)
	}
	if session.TrainingReady {
		t.Fatal("overlapping session is ready")
	}
	catalog, err := s.ListMaterials(context.Background(), 1, "N2", "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Materials) != 1 || catalog.Materials[0].TrainingReady {
		t.Fatalf("catalog readiness disagrees with session: %+v", catalog)
	}
}

func TestListeningCompletionCallbackRollsBack(t *testing.T) {
	t.Setenv("REQUIRE_TEST_DATABASE", "1")
	db := newMigratedPostgresTestDB(t)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'atomic@test.local','test')`); err != nil {
		t.Fatal(err)
	}
	id := listeningFixture(t, db, "Atomic completion")
	ctx := context.Background()
	s := NewListeningStore(db, func(ctx context.Context, tx *sql.Tx, uid int64, now time.Time) error {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET name='must rollback' WHERE id=$1`, uid); err != nil {
			return err
		}
		return fmt.Errorf("task completion unavailable")
	})
	for _, phase := range []string{"blind", "retest"} {
		if phase == "retest" {
			for _, stage := range []string{"intensive", "retest"} {
				if _, err := s.SaveProgress(ctx, 1, id, listening.ProgressRequest{Stage: stage, ExpectedCycle: 1}); err != nil {
					t.Fatal(err)
				}
			}
		}
		if _, err := s.SubmitAttempt(ctx, 1, id, listening.AttemptRequest{Phase: phase, Answers: listeningAnswer(id, 0), IdempotencyKey: "atomic-" + phase}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Complete(ctx, 1, id); err == nil {
		t.Fatal("callback failure was swallowed")
	}
	var stage, name string
	if err := db.QueryRow(`SELECT p.stage,u.name FROM listening_progress p JOIN users u ON u.id=p.user_id WHERE p.user_id=1 AND p.lesson_id=$1`, id).Scan(&stage, &name); err != nil {
		t.Fatal(err)
	}
	if stage != "retest" || name != "" {
		t.Fatalf("failed completion persisted: stage=%s name=%q", stage, name)
	}
	s.completeTask = nil
	if _, err := s.Complete(ctx, 1, id); err != nil {
		t.Fatalf("completion retry failed: %v", err)
	}
}

func TestListeningAudioSource(t *testing.T) {
	t.Setenv("REQUIRE_TEST_DATABASE", "1")
	db := newMigratedPostgresTestDB(t)
	id := listeningFixture(t, db, "Audio source")
	var audioID int64
	if err := db.QueryRow(`INSERT INTO audio_objects(bucket,object_key,kind,visibility,content_sha256,size_bytes,mime_type,metadata_json) VALUES('test','voice.wav','lesson','public','test',1,'audio/wav','{"voice":"Nanami"}') RETURNING id`).Scan(&audioID); err != nil {
		t.Fatal(err)
	}
	s := NewListeningStore(db)
	ctx := context.Background()
	for _, tc := range []struct {
		name, explicit string
		metadata       bool
		want           string
	}{{"unverified", "", false, "unknown"}, {"tts metadata", "", true, "synthetic"}, {"explicit human", "human", true, "human"}, {"explicit unknown", "unknown", true, "unknown"}, {"explicit synthetic", "synthetic", false, "synthetic"}} {
		t.Run(tc.name, func(t *testing.T) {
			var object any
			if tc.metadata {
				object = audioID
			}
			if _, err := db.Exec(`UPDATE lessons SET audio_object_id=$2,shadowing_config_json=jsonb_set(shadowing_config_json,'{audio_source}',to_jsonb($3::text)) WHERE id=$1`, id, object, tc.explicit); err != nil {
				t.Fatal(err)
			}
			session, err := s.GetSession(ctx, 1, id)
			if err != nil {
				t.Fatal(err)
			}
			catalog, err := s.ListMaterials(ctx, 1, "N2", "all")
			if err != nil {
				t.Fatal(err)
			}
			if session.AudioSource != tc.want || len(catalog.Materials) != 1 || catalog.Materials[0].AudioSource != tc.want {
				t.Fatalf("source session=%s catalog=%+v want=%s", session.AudioSource, catalog, tc.want)
			}
		})
	}
}

func TestListeningNewQuestionsCanRestartBlindAssessment(t *testing.T) {
	t.Setenv("REQUIRE_TEST_DATABASE", "1")
	db := newMigratedPostgresTestDB(t)
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(1,'new-questions@test.local','test')`); err != nil {
		t.Fatal(err)
	}
	s := NewListeningStore(db)
	ctx := context.Background()
	for _, initialStage := range []string{"intensive", "retest"} {
		t.Run(initialStage, func(t *testing.T) {
			id := listeningFixture(t, db, "New questions from "+initialStage)
			if _, err := db.Exec(`DELETE FROM listening_questions WHERE lesson_id=$1`, id); err != nil {
				t.Fatal(err)
			}
			if _, err := s.SaveProgress(ctx, 1, id, listening.ProgressRequest{Stage: "intensive", ExpectedCycle: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO listening_questions(id,lesson_id,prompt,options_json,correct_option_index,explanation) VALUES($1,$2,'明天怎么样？','["休息","上班"]',0,'休み表示休息。')`, fmt.Sprintf("q-%d", id), id); err != nil {
				t.Fatal(err)
			}
			if initialStage == "intensive" {
				_, err := s.SaveProgress(ctx, 1, id, listening.ProgressRequest{Stage: "retest", ExpectedCycle: 1})
				var typed *listening.Error
				if !errors.As(err, &typed) || typed.Status != 409 {
					t.Fatalf("entering retest without a blind attempt must fail: %v", err)
				}
			} else {
				// A previous server could admit a learner into retest before any
				// blind submission. That saved state also needs a recovery path.
				if _, err := db.Exec(`UPDATE listening_progress SET stage='retest' WHERE user_id=1 AND lesson_id=$1`, id); err != nil {
					t.Fatal(err)
				}
			}
			restarted, err := s.SaveProgress(ctx, 1, id, listening.ProgressRequest{Stage: "blind", ExpectedCycle: 1})
			if err != nil {
				t.Fatalf("cannot start newly available blind assessment: %v", err)
			}
			if restarted.Stage != "blind" || restarted.Cycle != 1 || restarted.DraftAnswers != nil {
				t.Fatalf("restart changed the assessment cycle or retained a draft: %+v", restarted)
			}
			if _, err := s.SubmitAttempt(ctx, 1, id, listening.AttemptRequest{Phase: "blind", Answers: listeningAnswer(id, 0), IdempotencyKey: "new-questions-" + initialStage}); err != nil {
				t.Fatal(err)
			}
			for _, nextStage := range []string{"intensive", "retest"} {
				if _, err := s.SaveProgress(ctx, 1, id, listening.ProgressRequest{Stage: nextStage, ExpectedCycle: 1}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.SubmitAttempt(ctx, 1, id, listening.AttemptRequest{Phase: "retest", Answers: listeningAnswer(id, 0), IdempotencyKey: "new-questions-retest-" + initialStage}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Complete(ctx, 1, id); err != nil {
				t.Fatalf("recovered training cannot complete: %v", err)
			}
		})
	}
}
