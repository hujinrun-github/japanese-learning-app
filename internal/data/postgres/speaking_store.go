package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"japanese-learning-app/internal/module/speaking"
	"japanese-learning-app/internal/store"
)

type SpeakingStore struct {
	db  queryer
	now func() time.Time
}

func NewSpeakingStore(db queryer, deps store.StoreDeps) *SpeakingStore {
	return &SpeakingStore{
		db:  db,
		now: currentTimeFunc(deps.Clock),
	}
}

func (s *SpeakingStore) ListAll(practiceType, level string, offset, limit int) ([]speaking.SpeakingMaterial, int, error) {
	where, args := buildSpeakingMaterialFilter(practiceType, level)

	total, err := countRows(context.Background(), s.db, `SELECT COUNT(*) FROM speaking_materials `+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.SpeakingStore.ListAll count: %w", err)
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(
		`SELECT id, type, title, text, COALESCE(audio_url_legacy, ''), jlpt_level
		 FROM speaking_materials
		 %s
		 ORDER BY updated_at DESC, id DESC
		 LIMIT $%d OFFSET $%d`,
		where,
		len(args)-1,
		len(args),
	)

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.SpeakingStore.ListAll query: %w", translateError(err))
	}
	defer rows.Close()

	items, err := scanSpeakingMaterials(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *SpeakingStore) InsertMaterial(m speaking.SpeakingMaterial) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(
		context.Background(),
		`INSERT INTO speaking_materials (type, title, text, jlpt_level, audio_url_legacy, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		m.Type,
		m.Title,
		m.Text,
		m.JLPTLevel,
		m.AudioURL,
		s.now().UTC(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("postgres.SpeakingStore.InsertMaterial: %w", translateError(err))
	}
	return id, nil
}

func (s *SpeakingStore) UpdateMaterial(m speaking.SpeakingMaterial) error {
	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE speaking_materials
		 SET type = $2,
		     title = $3,
		     text = $4,
		     jlpt_level = $5,
		     audio_url_legacy = $6,
		     updated_at = $7
		 WHERE id = $1`,
		m.ID,
		m.Type,
		m.Title,
		m.Text,
		m.JLPTLevel,
		m.AudioURL,
		s.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres.SpeakingStore.UpdateMaterial: %w", translateError(err))
	}
	return nil
}

func (s *SpeakingStore) DeleteMaterial(id int64) error {
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM speaking_materials WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres.SpeakingStore.DeleteMaterial: %w", translateError(err))
	}
	return nil
}

func (s *SpeakingStore) ListAllRecords(userID int64, offset, limit int) ([]speaking.SpeakingRecord, int, error) {
	where := ""
	args := []any{}
	if userID > 0 {
		where = "WHERE user_id = $1"
		args = append(args, userID)
	}

	totalQuery := `SELECT COUNT(*) FROM speaking_records`
	if where != "" {
		totalQuery += " " + where
	}
	total, err := countRows(context.Background(), s.db, totalQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.SpeakingStore.ListAllRecords count: %w", err)
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(
		`SELECT id, user_id, type, material_id, score, audio_ref_legacy, practiced_at
		 FROM speaking_records
		 %s
		 ORDER BY id DESC
		 LIMIT $%d OFFSET $%d`,
		where,
		len(args)-1,
		len(args),
	)

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres.SpeakingStore.ListAllRecords query: %w", translateError(err))
	}
	defer rows.Close()

	var records []speaking.SpeakingRecord
	for rows.Next() {
		var record speaking.SpeakingRecord
		if err := rows.Scan(
			&record.ID,
			&record.UserID,
			&record.Type,
			&record.MaterialID,
			&record.Score,
			&record.AudioRef,
			&record.PracticedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("postgres.SpeakingStore.ListAllRecords scan: %w", translateError(err))
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("postgres.SpeakingStore.ListAllRecords rows: %w", translateError(err))
	}
	return records, total, nil
}

func buildSpeakingMaterialFilter(practiceType, level string) (string, []any) {
	var clauses []string
	var args []any
	if practiceType != "" {
		args = append(args, practiceType)
		clauses = append(clauses, fmt.Sprintf("type = $%d", len(args)))
	}
	if level != "" {
		args = append(args, level)
		clauses = append(clauses, fmt.Sprintf("jlpt_level = $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func scanSpeakingMaterials(rows *sql.Rows) ([]speaking.SpeakingMaterial, error) {
	items := make([]speaking.SpeakingMaterial, 0)
	for rows.Next() {
		var item speaking.SpeakingMaterial
		if err := rows.Scan(
			&item.ID,
			&item.Type,
			&item.Title,
			&item.Text,
			&item.AudioURL,
			&item.JLPTLevel,
		); err != nil {
			return nil, fmt.Errorf("postgres.scanSpeakingMaterials scan: %w", translateError(err))
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.scanSpeakingMaterials rows: %w", translateError(err))
	}
	return items, nil
}
