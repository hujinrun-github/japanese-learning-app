package data

import (
	"database/sql"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/speaking"
)

// SpeakingStore 实现口语练习数据访问，对应 speaking_records 表。
type SpeakingStore struct {
	db *sql.DB
}

// NewSpeakingStore 创建 SpeakingStore 实例。
func NewSpeakingStore(db *sql.DB) *SpeakingStore {
	return &SpeakingStore{db: db}
}

// SaveRecord 保存一次口语练习记录，返回自动生成的记录 ID。
func (s *SpeakingStore) SaveRecord(r speaking.SpeakingRecord) (int64, error) {
	slog.Debug("SpeakingStore.SaveRecord called", "user_id", r.UserID, "material_id", r.MaterialID)

	result, err := s.db.Exec(
		`INSERT INTO speaking_records (user_id, type, material_id, score, audio_ref, practiced_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.UserID, r.Type, r.MaterialID, r.Score, r.AudioRef, formatSQLiteTime(r.PracticedAt),
	)
	if err != nil {
		slog.Error("failed to insert speaking_record", "err", err, "user_id", r.UserID)
		return 0, fmt.Errorf("data.SpeakingStore.SaveRecord: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("data.SpeakingStore.SaveRecord LastInsertId: %w", err)
	}

	slog.Debug("SpeakingStore.SaveRecord done", "record_id", id, "user_id", r.UserID)
	return id, nil
}

// ListRecords 查询用户所有口语练习记录，按 practiced_at 倒序。
func (s *SpeakingStore) ListRecords(userID int64) ([]speaking.SpeakingRecord, error) {
	slog.Debug("SpeakingStore.ListRecords called", "user_id", userID)

	rows, err := s.db.Query(
		`SELECT id, user_id, type, material_id, score, audio_ref, practiced_at
		 FROM speaking_records WHERE user_id = ?
		 ORDER BY practiced_at DESC`,
		userID,
	)
	if err != nil {
		slog.Error("failed to query speaking_records", "err", err, "user_id", userID)
		return nil, fmt.Errorf("data.SpeakingStore.ListRecords query: %w", err)
	}
	defer rows.Close()

	var records []speaking.SpeakingRecord
	for rows.Next() {
		var r speaking.SpeakingRecord
		var practicedAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.Type, &r.MaterialID, &r.Score, &r.AudioRef, &practicedAt); err != nil {
			slog.Error("failed to scan speaking_record row", "err", err)
			return nil, fmt.Errorf("data.SpeakingStore.ListRecords scan: %w", err)
		}
		r.PracticedAt, err = parseSQLiteTime(practicedAt)
		if err != nil {
			return nil, fmt.Errorf("data.SpeakingStore.ListRecords parse practiced_at: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows iteration error", "err", err)
		return nil, fmt.Errorf("data.SpeakingStore.ListRecords rows: %w", err)
	}

	slog.Debug("SpeakingStore.ListRecords done", "user_id", userID, "count", len(records))
	return records, nil
}

// GetRecord 按记录 ID 查询口语练习记录，不存在时返回 error。
func (s *SpeakingStore) GetRecord(id int64) (*speaking.SpeakingRecord, error) {
	slog.Debug("SpeakingStore.GetRecord called", "record_id", id)

	row := s.db.QueryRow(
		`SELECT id, user_id, type, material_id, score, audio_ref, practiced_at
		 FROM speaking_records WHERE id = ?`, id,
	)

	var r speaking.SpeakingRecord
	var practicedAt string
	err := row.Scan(&r.ID, &r.UserID, &r.Type, &r.MaterialID, &r.Score, &r.AudioRef, &practicedAt)
	if err == sql.ErrNoRows {
		slog.Debug("speaking_record not found", "record_id", id)
		return nil, fmt.Errorf("data.SpeakingStore.GetRecord %d: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		slog.Error("failed to scan speaking_record", "err", err, "record_id", id)
		return nil, fmt.Errorf("data.SpeakingStore.GetRecord: %w", err)
	}

	r.PracticedAt, err = parseSQLiteTime(practicedAt)
	if err != nil {
		return nil, fmt.Errorf("data.SpeakingStore.GetRecord parse practiced_at: %w", err)
	}

	slog.Debug("SpeakingStore.GetRecord done", "record_id", id, "user_id", r.UserID)
	return &r, nil
}

// ListMaterials 查询口语练习素材，支持按 type 和 level 过滤。
func (s *SpeakingStore) ListMaterials(practiceType, level string) ([]speaking.SpeakingMaterial, error) {
	slog.Debug("SpeakingStore.ListMaterials called", "type", practiceType, "level", level)

	query := `SELECT id, type, title, text, audio_url, jlpt_level FROM speaking_materials WHERE 1=1`
	var args []any

	if practiceType != "" {
		query += " AND type = ?"
		args = append(args, practiceType)
	}
	if level != "" {
		query += " AND jlpt_level = ?"
		args = append(args, level)
	}
	query += " ORDER BY id ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		slog.Error("failed to query speaking_materials", "err", err)
		return nil, fmt.Errorf("data.SpeakingStore.ListMaterials query: %w", err)
	}
	defer rows.Close()

	materials := make([]speaking.SpeakingMaterial, 0)
	for rows.Next() {
		var m speaking.SpeakingMaterial
		if err := rows.Scan(&m.ID, &m.Type, &m.Title, &m.Text, &m.AudioURL, &m.JLPTLevel); err != nil {
			slog.Error("failed to scan speaking_material row", "err", err)
			return nil, fmt.Errorf("data.SpeakingStore.ListMaterials scan: %w", err)
		}
		materials = append(materials, m)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows iteration error", "err", err)
		return nil, fmt.Errorf("data.SpeakingStore.ListMaterials rows: %w", err)
	}

	slog.Debug("SpeakingStore.ListMaterials done", "count", len(materials))
	return materials, nil
}

// GetMaterialByID 按 ID 查询单个口语练习素材，不存在时返回 nil, nil。
func (s *SpeakingStore) GetMaterialByID(id int64) (*speaking.SpeakingMaterial, error) {
	slog.Debug("SpeakingStore.GetMaterialByID called", "id", id)

	row := s.db.QueryRow(
		`SELECT id, type, title, text, audio_url, jlpt_level FROM speaking_materials WHERE id = ?`, id,
	)

	var m speaking.SpeakingMaterial
	err := row.Scan(&m.ID, &m.Type, &m.Title, &m.Text, &m.AudioURL, &m.JLPTLevel)
	if err == sql.ErrNoRows {
		slog.Debug("speaking_material not found", "id", id)
		return nil, nil
	}
	if err != nil {
		slog.Error("failed to scan speaking_material", "err", err, "id", id)
		return nil, fmt.Errorf("data.SpeakingStore.GetMaterialByID: %w", err)
	}

	slog.Debug("SpeakingStore.GetMaterialByID done", "id", id, "title", m.Title)
	return &m, nil
}
