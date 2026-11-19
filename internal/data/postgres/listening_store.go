package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/listening"
)

// ListeningStore keeps assessment and review state independent of shadowing.
// Progress rows serialize mutations for one learner/material; unique constraints
// preserve the first answer even when requests race or are retried.
type ListeningStore struct {
	db           *sql.DB
	now          func() time.Time
	completeTask func(context.Context, *sql.Tx, int64, time.Time) error
}

func NewListeningStore(db *sql.DB, completion ...func(context.Context, *sql.Tx, int64, time.Time) error) *ListeningStore {
	s := &ListeningStore{db: db, now: time.Now}
	if len(completion) > 0 {
		s.completeTask = completion[0]
	}
	return s
}

func (s *ListeningStore) ListMaterials(ctx context.Context, uid int64, level, status string) (listening.Materials, error) {
	out := listening.Materials{Materials: []listening.Summary{}}
	if level != "" && level != "N5" && level != "N4" && level != "N3" && level != "N2" && level != "N1" {
		return out, listening.Invalid("JLPT 等级无效")
	}
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "new" && status != "practiced" && status != "due" {
		return out, listening.Invalid("练习状态无效")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT l.id,l.title,l.jlpt_level,
	 COALESCE(NULLIF(l.shadowing_config_json->>'audio_url',''),CASE WHEN COALESCE(l.shadowing_config_json->>'media_type','audio')<>'video' THEN NULLIF(l.shadowing_config_json->>'media_url','') END,CASE WHEN ao.id IS NOT NULL THEN '/api/v1/audio/'||ao.id::text||'/stream' ELSE '' END),
	 COALESCE(NULLIF(l.shadowing_config_json->>'video_url',''),CASE WHEN l.shadowing_config_json->>'media_type'='video' THEN NULLIF(l.shadowing_config_json->>'media_url','') END,CASE WHEN vo.id IS NOT NULL THEN '/api/v1/videos/'||vo.id::text||'/stream' ELSE '' END),
	 CASE WHEN l.shadowing_config_json->>'audio_source' IN ('synthetic','human','unknown') THEN l.shadowing_config_json->>'audio_source' WHEN ao.metadata_json ? 'tts_provider' OR ao.metadata_json ? 'voice' THEN 'synthetic' ELSE 'unknown' END,
	 COALESCE(ss.duration_ms,0),ss.sentence_count,COALESCE(ss.valid,false),qq.question_count,
	 p.user_id IS NOT NULL,COALESCE(p.stage,'blind'),COALESCE(p.sentence_index,0),COALESCE(p.position_ms,0),COALESCE(p.bookmarks_json,'[]'::jsonb)::text,
	 COALESCE(p.cycle,1),COALESCE(p.review_step,0),COALESCE(p.completed_count,0),p.due_at
	 FROM lessons l
	 LEFT JOIN audio_objects ao ON ao.id=l.audio_object_id AND ao.deleted_at IS NULL AND ao.visibility='public'
	 LEFT JOIN video_objects vo ON vo.id=l.video_object_id AND vo.deleted_at IS NULL AND vo.visibility='public'
	 CROSS JOIN LATERAL(SELECT COUNT(*) AS sentence_count,MAX(end_ms) AS duration_ms,BOOL_AND(end_ms>start_ms AND start_ms>=COALESCE(previous_end,0) AND jsonb_array_length(tokens)>0) AS valid FROM (SELECT start_ms,end_ms,tokens,LAG(end_ms) OVER(ORDER BY position) AS previous_end FROM lesson_sentences WHERE lesson_id=l.id) segments) ss
	 CROSS JOIN LATERAL(SELECT COUNT(*) AS question_count FROM listening_questions WHERE lesson_id=l.id AND published) qq
	 LEFT JOIN listening_progress p ON p.lesson_id=l.id AND p.user_id=$1
	 WHERE ($2='' OR l.jlpt_level=$2) AND ss.sentence_count>0
	 ORDER BY l.id`, uid, level)
	if err != nil {
		return out, fmt.Errorf("postgres.ListeningStore.ListMaterials query: %w", err)
	}
	defer rows.Close()
	now := s.now()
	for rows.Next() {
		var item listening.Summary
		var valid, practiced bool
		var bookmarks string
		if err := rows.Scan(&item.ID, &item.Title, &item.JLPTLevel, &item.AudioURL, &item.VideoURL, &item.AudioSource, &item.DurationMS, &item.SentenceCount, &valid, &item.QuestionCount, &practiced, &item.Progress.Stage, &item.Progress.SentenceIndex, &item.Progress.PositionMS, &bookmarks, &item.Progress.Cycle, &item.Progress.ReviewStep, &item.Progress.CompletedCount, &item.Progress.DueAt); err != nil {
			return out, fmt.Errorf("postgres.ListeningStore.ListMaterials scan: %w", err)
		}
		if strings.TrimSpace(item.AudioURL) == "" && strings.TrimSpace(item.VideoURL) == "" {
			continue
		}
		if err := json.Unmarshal([]byte(bookmarks), &item.Progress.Bookmarks); err != nil {
			return out, fmt.Errorf("postgres.ListeningStore.ListMaterials bookmarks: %w", err)
		}
		item.TrainingReady = valid && item.QuestionCount > 0
		item.Status = "new"
		if practiced {
			item.Status = "practiced"
		}
		due := item.Progress.DueAt != nil && !item.Progress.DueAt.After(now)
		if due {
			item.Status = "due"
		}
		out.Stats.Total++
		if practiced {
			out.Stats.Practiced++
		} else {
			out.Stats.New++
		}
		if due {
			out.Stats.Due++
		}
		if status == "all" || (status == "new" && !practiced) || (status == "practiced" && practiced) || (status == "due" && due) {
			out.Materials = append(out.Materials, item)
		}
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("postgres.ListeningStore.ListMaterials rows: %w", err)
	}
	return out, nil
}

func loadListeningLesson(q queryer, id int64) (*lesson.Lesson, error) {
	l, err := NewLessonStore(q).GetDetail(id)
	if errors.Is(err, sql.ErrNoRows) || l == nil && err == nil {
		return nil, listening.NotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("postgres.ListeningStore lesson: %w", err)
	}
	if strings.TrimSpace(l.AudioURL) == "" && strings.TrimSpace(l.VideoURL) == "" {
		return nil, listening.NotFound()
	}
	return l, nil
}

func listeningReady(l *lesson.Lesson, questions []listening.Question) bool {
	if len(questions) == 0 || len(l.Sentences) == 0 {
		return false
	}
	previousEnd := int64(0)
	for _, sentence := range l.Sentences {
		if sentence.StartMS < previousEnd || sentence.EndMS <= sentence.StartMS || len(sentence.Tokens) == 0 {
			return false
		}
		previousEnd = sentence.EndMS
	}
	for _, q := range questions {
		if !listeningSentenceExists(l, q.EvidenceSentenceIndex) {
			return false
		}
	}
	return true
}

func listeningSentenceExists(l *lesson.Lesson, index int) bool {
	for _, sentence := range l.Sentences {
		if sentence.Index == index {
			return true
		}
	}
	return false
}

func loadListeningQuestions(ctx context.Context, q queryer, id int64) ([]listening.Question, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,type,prompt,options_json::text,correct_option_index,explanation,evidence_sentence_index,version FROM listening_questions WHERE lesson_id=$1 AND published ORDER BY position,id`, id)
	if err != nil {
		return nil, fmt.Errorf("postgres.ListeningStore questions query: %w", err)
	}
	defer rows.Close()
	out := []listening.Question{}
	for rows.Next() {
		var item listening.Question
		var options string
		if err := rows.Scan(&item.ID, &item.Type, &item.Prompt, &options, &item.CorrectOptionIndex, &item.Explanation, &item.EvidenceSentenceIndex, &item.Version); err != nil {
			return nil, fmt.Errorf("postgres.ListeningStore questions scan: %w", err)
		}
		if err := json.Unmarshal([]byte(options), &item.Options); err != nil {
			return nil, fmt.Errorf("postgres.ListeningStore questions options: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.ListeningStore questions rows: %w", err)
	}
	return out, nil
}

func defaultListeningProgress() listening.Progress {
	return listening.Progress{Stage: "blind", Bookmarks: []int{}, Cycle: 1}
}

func loadListeningProgress(ctx context.Context, q queryer, uid, id int64, lock bool) (listening.Progress, error) {
	p := defaultListeningProgress()
	query := `SELECT stage,sentence_index,position_ms,bookmarks_json::text,draft_answers_json::text,cycle,review_step,completed_count,due_at FROM listening_progress WHERE user_id=$1 AND lesson_id=$2`
	if lock {
		query += " FOR UPDATE"
	}
	var bookmarks string
	var draft sql.NullString
	err := q.QueryRowContext(ctx, query, uid, id).Scan(&p.Stage, &p.SentenceIndex, &p.PositionMS, &bookmarks, &draft, &p.Cycle, &p.ReviewStep, &p.CompletedCount, &p.DueAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, fmt.Errorf("postgres.ListeningStore progress query: %w", err)
	}
	if err := json.Unmarshal([]byte(bookmarks), &p.Bookmarks); err != nil {
		return p, fmt.Errorf("postgres.ListeningStore progress bookmarks: %w", err)
	}
	if draft.Valid {
		if err := json.Unmarshal([]byte(draft.String), &p.DraftAnswers); err != nil {
			return p, fmt.Errorf("postgres.ListeningStore progress draft: %w", err)
		}
	}
	return p, nil
}

func lockListeningProgress(ctx context.Context, tx *sql.Tx, uid, id int64) (listening.Progress, error) {
	if _, err := tx.ExecContext(ctx, `INSERT INTO listening_progress(user_id,lesson_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, uid, id); err != nil {
		return listening.Progress{}, fmt.Errorf("postgres.ListeningStore ensure progress: %w", err)
	}
	return loadListeningProgress(ctx, tx, uid, id, true)
}

func saveListeningProgress(ctx context.Context, tx *sql.Tx, uid, id int64, p listening.Progress) error {
	bookmarks, err := json.Marshal(p.Bookmarks)
	if err != nil {
		return fmt.Errorf("postgres.ListeningStore encode bookmarks: %w", err)
	}
	var draft any
	if p.DraftAnswers != nil {
		encoded, err := json.Marshal(p.DraftAnswers)
		if err != nil {
			return fmt.Errorf("postgres.ListeningStore encode draft: %w", err)
		}
		draft = string(encoded)
	}
	_, err = tx.ExecContext(ctx, `UPDATE listening_progress SET stage=$3,sentence_index=$4,position_ms=$5,bookmarks_json=$6::jsonb,draft_answers_json=$7::jsonb,cycle=$8,review_step=$9,completed_count=$10,due_at=$11,updated_at=now() WHERE user_id=$1 AND lesson_id=$2`, uid, id, p.Stage, p.SentenceIndex, p.PositionMS, string(bookmarks), draft, p.Cycle, p.ReviewStep, p.CompletedCount, p.DueAt)
	if err != nil {
		return fmt.Errorf("postgres.ListeningStore save progress: %w", err)
	}
	return nil
}

func loadListeningAttempts(ctx context.Context, q queryer, uid, id int64, cycle int) ([]listening.AttemptResult, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,result_json::text,created_at FROM listening_attempts WHERE user_id=$1 AND lesson_id=$2 AND cycle=$3 ORDER BY id`, uid, id, cycle)
	if err != nil {
		return nil, fmt.Errorf("postgres.ListeningStore attempts query: %w", err)
	}
	defer rows.Close()
	out := []listening.AttemptResult{}
	for rows.Next() {
		var item listening.AttemptResult
		var raw string
		var attemptID int64
		var created time.Time
		if err := rows.Scan(&attemptID, &raw, &created); err != nil {
			return nil, fmt.Errorf("postgres.ListeningStore attempt scan: %w", err)
		}
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, fmt.Errorf("postgres.ListeningStore decode attempt: %w", err)
		}
		item.ID = attemptID
		item.CreatedAt = created.UTC()
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.ListeningStore attempts rows: %w", err)
	}
	return out, nil
}

func (s *ListeningStore) GetSession(ctx context.Context, uid, id int64) (listening.Session, error) {
	out := listening.Session{}
	l, err := loadListeningLesson(s.db, id)
	if err != nil {
		return out, err
	}
	questions, err := loadListeningQuestions(ctx, s.db, id)
	if err != nil {
		return out, err
	}
	p, err := loadListeningProgress(ctx, s.db, uid, id, false)
	if err != nil {
		return out, err
	}
	attempts, err := loadListeningAttempts(ctx, s.db, uid, id, p.Cycle)
	if err != nil {
		return out, err
	}
	var source string
	if err := s.db.QueryRowContext(ctx, `SELECT CASE WHEN l.shadowing_config_json->>'audio_source' IN ('synthetic','human','unknown') THEN l.shadowing_config_json->>'audio_source' WHEN ao.metadata_json ? 'tts_provider' OR ao.metadata_json ? 'voice' THEN 'synthetic' ELSE 'unknown' END FROM lessons l LEFT JOIN audio_objects ao ON ao.id=l.audio_object_id AND ao.deleted_at IS NULL WHERE l.id=$1`, id).Scan(&source); err != nil {
		return out, fmt.Errorf("postgres.ListeningStore audio source: %w", err)
	}
	return listening.Session{AudioSource: source, Lesson: l, Questions: questions, Progress: p, Attempts: attempts, TrainingReady: listeningReady(l, questions)}, nil
}

func validateListeningProgress(l *lesson.Lesson, questions []listening.Question, req listening.ProgressRequest) error {
	if req.ExpectedCycle < 1 {
		return listening.Invalid("请提供当前训练轮次")
	}
	if req.Stage != "blind" && req.Stage != "intensive" && req.Stage != "retest" && req.Stage != "completed" {
		return listening.Invalid("训练阶段无效")
	}
	if !listeningSentenceExists(l, req.SentenceIndex) || req.PositionMS < 0 {
		return listening.Invalid("播放位置或句子无效")
	}
	end := int64(0)
	for _, sentence := range l.Sentences {
		if sentence.EndMS > end {
			end = sentence.EndMS
		}
	}
	if req.PositionMS > end {
		return listening.Invalid("播放位置超出材料时长")
	}
	if len(req.Bookmarks) > len(l.Sentences) {
		return listening.Invalid("书签过多")
	}
	seen := map[int]bool{}
	for _, index := range req.Bookmarks {
		if !listeningSentenceExists(l, index) || seen[index] {
			return listening.Invalid("书签句子无效或重复")
		}
		seen[index] = true
	}
	if req.DraftAnswers != nil {
		if req.DraftAnswers.Phase != req.Stage || (req.Stage != "blind" && req.Stage != "retest") {
			return listening.Invalid("草稿阶段无效")
		}
		if err := validateListeningAnswers(questions, req.DraftAnswers.Answers, false); err != nil {
			return err
		}
	}
	return nil
}

func validateListeningAnswers(questions []listening.Question, answers []listening.Answer, complete bool) error {
	if len(answers) > len(questions) || (complete && len(answers) != len(questions)) {
		return listening.Invalid("请回答所有题目后提交")
	}
	byID := map[string]listening.Question{}
	for _, q := range questions {
		byID[q.ID] = q
	}
	seen := map[string]bool{}
	for _, answer := range answers {
		q, ok := byID[answer.QuestionID]
		if !ok || seen[answer.QuestionID] || answer.OptionIndex == nil || *answer.OptionIndex < 0 || *answer.OptionIndex >= len(q.Options) {
			return listening.Invalid("答案包含无效题目、重复题目或选项")
		}
		seen[answer.QuestionID] = true
	}
	return nil
}

func hasListeningPhase(attempts []listening.AttemptResult, phase string) bool {
	for _, a := range attempts {
		if a.Phase == phase {
			return true
		}
	}
	return false
}

func (s *ListeningStore) SaveProgress(ctx context.Context, uid, id int64, req listening.ProgressRequest) (listening.Progress, error) {
	l, err := loadListeningLesson(s.db, id)
	if err != nil {
		return listening.Progress{}, err
	}
	questions, err := loadListeningQuestions(ctx, s.db, id)
	if err != nil {
		return listening.Progress{}, err
	}
	if err := validateListeningProgress(l, questions, req); err != nil {
		return listening.Progress{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return listening.Progress{}, fmt.Errorf("postgres.ListeningStore progress begin: %w", err)
	}
	defer tx.Rollback()
	p, err := lockListeningProgress(ctx, tx, uid, id)
	if err != nil {
		return p, err
	}
	if req.ExpectedCycle != p.Cycle {
		return p, listening.Conflict("训练轮次已变化，请重新读取进度")
	}
	if req.Restart && (p.Stage != "completed" || req.Stage != "blind") {
		return p, listening.Conflict("只有已完成的训练可以开始新一轮")
	}
	attempts, err := loadListeningAttempts(ctx, tx, uid, id, p.Cycle)
	if err != nil {
		return p, err
	}
	if req.Stage != p.Stage {
		switch {
		case p.Stage == "completed" && req.Stage == "blind":
			if !req.Restart {
				return p, listening.Conflict("训练已经完成，请明确选择再练一轮")
			}
			p.Cycle++
			attempts = nil
		case (p.Stage == "intensive" || p.Stage == "retest") && req.Stage == "blind" && listeningReady(l, questions) && !hasListeningPhase(attempts, "blind"):
			// Questions may be published after a learner started an audio-only
			// session. Admit its first assessment without creating another cycle.
			p.DraftAnswers = nil
		case p.Stage == "blind" && req.Stage == "intensive":
			if listeningReady(l, questions) && !hasListeningPhase(attempts, "blind") {
				return p, listening.Conflict("请先完成首次盲听答题")
			}
		case p.Stage == "intensive" && req.Stage == "retest":
			if !listeningReady(l, questions) {
				return p, listening.Conflict("此材料仅支持精听，暂无完整理解训练")
			}
			if !hasListeningPhase(attempts, "blind") {
				return p, listening.Conflict("理解题已就绪，请先开始盲听答题")
			}
		default:
			return p, listening.Conflict("请按盲听、精听、复测的顺序训练")
		}
	}
	if req.DraftAnswers != nil && hasListeningPhase(attempts, req.DraftAnswers.Phase) {
		return p, listening.Conflict("本阶段已提交，无法覆盖答案")
	}
	previousStage := p.Stage
	if req.DraftAnswers != nil {
		if p.DraftAnswers != nil && p.DraftAnswers.Phase == req.DraftAnswers.Phase && p.DraftAnswers.Assisted != nil && *p.DraftAnswers.Assisted {
			assisted := true
			req.DraftAnswers.Assisted = &assisted
		}
		p.DraftAnswers = req.DraftAnswers
	} else if previousStage != req.Stage {
		p.DraftAnswers = nil
	}
	p.Stage = req.Stage
	p.SentenceIndex = req.SentenceIndex
	p.PositionMS = req.PositionMS
	p.Bookmarks = req.Bookmarks
	if p.Bookmarks == nil {
		p.Bookmarks = []int{}
	}
	if err := saveListeningProgress(ctx, tx, uid, id, p); err != nil {
		return p, err
	}
	if err := tx.Commit(); err != nil {
		return p, fmt.Errorf("postgres.ListeningStore progress commit: %w", err)
	}
	return p, nil
}

func listeningRequestHash(id int64, req listening.AttemptRequest) (string, error) {
	answers := append([]listening.Answer{}, req.Answers...)
	sort.Slice(answers, func(i, j int) bool { return answers[i].QuestionID < answers[j].QuestionID })
	req.Answers = answers
	req.IdempotencyKey = ""
	raw, err := json.Marshal(struct {
		LessonID int64
		Request  listening.AttemptRequest
	}{id, req})
	if err != nil {
		return "", fmt.Errorf("postgres.ListeningStore request hash: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s *ListeningStore) SubmitAttempt(ctx context.Context, uid, id int64, req listening.AttemptRequest) (listening.AttemptResult, error) {
	out := listening.AttemptResult{}
	if (req.Phase != "blind" && req.Phase != "retest") || len(req.IdempotencyKey) == 0 || len(req.IdempotencyKey) > 120 || strings.TrimSpace(req.IdempotencyKey) != req.IdempotencyKey {
		return out, listening.Invalid("提交阶段或请求标识无效")
	}
	l, err := loadListeningLesson(s.db, id)
	if err != nil {
		return out, err
	}
	questions, err := loadListeningQuestions(ctx, s.db, id)
	if err != nil {
		return out, err
	}
	if !listeningReady(l, questions) {
		return out, listening.Invalid("此材料缺少有效音频时间轴或理解题，请先选择其他材料")
	}
	if err := validateListeningAnswers(questions, req.Answers, true); err != nil {
		return out, err
	}
	hash, err := listeningRequestHash(id, req)
	if err != nil {
		return out, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, fmt.Errorf("postgres.ListeningStore attempt begin: %w", err)
	}
	defer tx.Rollback()
	p, err := lockListeningProgress(ctx, tx, uid, id)
	if err != nil {
		return out, err
	}
	// Same-material retries wait on the progress lock before inspecting the key.
	var oldHash, oldJSON string
	var oldID int64
	var oldCreated time.Time
	err = tx.QueryRowContext(ctx, `SELECT id,request_hash,result_json::text,created_at FROM listening_attempts WHERE user_id=$1 AND idempotency_key=$2`, uid, req.IdempotencyKey).Scan(&oldID, &oldHash, &oldJSON, &oldCreated)
	if err == nil {
		if hash != oldHash {
			return out, listening.Conflict("请求标识已用于不同答案")
		}
		if err := json.Unmarshal([]byte(oldJSON), &out); err != nil {
			return out, fmt.Errorf("postgres.ListeningStore retry decode: %w", err)
		}
		out.ID = oldID
		out.CreatedAt = oldCreated.UTC()
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, fmt.Errorf("postgres.ListeningStore retry query: %w", err)
	}
	attempts, err := loadListeningAttempts(ctx, tx, uid, id, p.Cycle)
	if err != nil {
		return out, err
	}
	if hasListeningPhase(attempts, req.Phase) {
		return out, listening.Conflict("本轮本阶段已提交，首次结果会被保留")
	}
	if req.Phase == "blind" && p.Stage != "blind" || req.Phase == "retest" && (p.Stage != "retest" || !hasListeningPhase(attempts, "blind")) {
		return out, listening.Conflict("当前训练阶段不能提交这次答案")
	}
	assisted := req.Assisted
	if p.DraftAnswers != nil && p.DraftAnswers.Phase == req.Phase && p.DraftAnswers.Assisted != nil && *p.DraftAnswers.Assisted {
		assisted = true
	}
	out = listening.AttemptResult{Cycle: p.Cycle, Phase: req.Phase, Assisted: assisted, Total: len(questions), Results: []listening.AnswerResult{}, CreatedAt: s.now().UTC().Truncate(time.Microsecond)}
	byID := map[string]int{}
	for _, a := range req.Answers {
		byID[a.QuestionID] = *a.OptionIndex
	}
	for _, q := range questions {
		answer := byID[q.ID]
		correct := answer == q.CorrectOptionIndex
		if correct {
			out.CorrectCount++
		}
		out.Results = append(out.Results, listening.AnswerResult{QuestionID: q.ID, OptionIndex: answer, CorrectOptionIndex: q.CorrectOptionIndex, Correct: correct, Explanation: q.Explanation, EvidenceSentenceIndex: q.EvidenceSentenceIndex})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return out, fmt.Errorf("postgres.ListeningStore encode result: %w", err)
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO listening_attempts(user_id,lesson_id,cycle,phase,assisted,idempotency_key,request_hash,result_json,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9) ON CONFLICT DO NOTHING RETURNING id`, uid, id, p.Cycle, req.Phase, out.Assisted, req.IdempotencyKey, hash, string(raw), out.CreatedAt).Scan(&out.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, listening.Conflict("此请求已提交，请刷新训练记录")
	}
	if err != nil {
		return out, fmt.Errorf("postgres.ListeningStore insert attempt: %w", err)
	}
	p.DraftAnswers = nil
	if err := saveListeningProgress(ctx, tx, uid, id, p); err != nil {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return out, fmt.Errorf("postgres.ListeningStore attempt commit: %w", err)
	}
	slog.Debug("listening attempt saved", "user_id", uid, "lesson_id", id, "cycle", p.Cycle, "phase", req.Phase, "correct", out.CorrectCount, "total", out.Total)
	return out, nil
}

func (s *ListeningStore) Complete(ctx context.Context, uid, id int64) (listening.Progress, error) {
	l, err := loadListeningLesson(s.db, id)
	if err != nil {
		return listening.Progress{}, err
	}
	questions, err := loadListeningQuestions(ctx, s.db, id)
	if err != nil {
		return listening.Progress{}, err
	}
	if !listeningReady(l, questions) {
		return listening.Progress{}, listening.Conflict("此材料暂不支持完整听力训练")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return listening.Progress{}, fmt.Errorf("postgres.ListeningStore complete begin: %w", err)
	}
	defer tx.Rollback()
	p, err := lockListeningProgress(ctx, tx, uid, id)
	if err != nil {
		return p, err
	}
	if p.Stage == "completed" {
		return p, nil
	}
	attempts, err := loadListeningAttempts(ctx, tx, uid, id, p.Cycle)
	if err != nil {
		return p, err
	}
	if p.Stage != "retest" || !hasListeningPhase(attempts, "blind") || !hasListeningPhase(attempts, "retest") {
		return p, listening.Conflict("请先完成盲听、精听和复测")
	}
	now := s.now()
	days := 1
	passed := false
	for _, a := range attempts {
		if a.Phase == "retest" && a.CorrectCount == a.Total && !a.Assisted {
			passed = true
			if (p.DueAt == nil || !p.DueAt.After(now)) && p.ReviewStep < 3 {
				p.ReviewStep++
			}
			switch p.ReviewStep {
			case 2:
				days = 3
			case 3:
				days = 7
			}
		}
	}
	if !passed {
		p.ReviewStep = 0
	}
	due := now.Add(time.Duration(days) * 24 * time.Hour)
	// Repeating before the next review date improves fluency but is not evidence
	// of retention. Keep that review's existing interval until it becomes due.
	if !passed || p.DueAt == nil || !p.DueAt.After(now) {
		p.DueAt = &due
	}
	p.Stage = "completed"
	p.CompletedCount++
	p.DraftAnswers = nil
	if err := saveListeningProgress(ctx, tx, uid, id, p); err != nil {
		return p, err
	}
	if s.completeTask != nil {
		if err := s.completeTask(ctx, tx, uid, now); err != nil {
			return p, fmt.Errorf("postgres.ListeningStore complete study task: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return p, fmt.Errorf("postgres.ListeningStore complete commit: %w", err)
	}
	slog.Debug("listening training completed", "user_id", uid, "lesson_id", id, "cycle", p.Cycle, "review_step", p.ReviewStep, "due_at", due)
	return p, nil
}
