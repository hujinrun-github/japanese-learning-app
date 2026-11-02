package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"japanese-learning-app/internal/module/notesync"
	"japanese-learning-app/internal/store"
)

type NoteSyncStore struct {
	db  queryer
	now func() time.Time
}

func NewNoteSyncStore(db queryer, deps store.StoreDeps) *NoteSyncStore {
	return &NoteSyncStore{db: db, now: currentTimeFunc(deps.Clock)}
}

const syncTargetColumns = `id, user_id, type, name, data_source_id, token_env, title_property,
    tags_property, flow_id_property, COALESCE(array_to_json(sync_tags)::text, '[]'), enabled, auto_push, created_at, updated_at`

const syncStateColumns = `note_id, target_id, external_page_id, external_url, local_hash, external_hash,
    last_direction, last_synced_at, last_error, pending_delete, updated_at`

func (s *NoteSyncStore) CreateTarget(target *notesync.SyncTarget) error {
	tags := normalizeStringSlice(target.SyncTags)
	var id int64
	err := s.db.QueryRowContext(context.Background(), `INSERT INTO note_sync_targets
        (user_id, type, name, data_source_id, token_env, title_property, tags_property, flow_id_property, sync_tags, enabled, auto_push)
        VALUES ($1, 'notion', $2, $3, $4, $5, $6, $7, $8::text[], $9, $10) RETURNING id`,
		target.UserID, target.Name, target.DataSourceID, target.TokenEnv, target.TitleProperty, target.TagsProperty, target.FlowIDProperty, tags, target.Enabled, target.AutoPush).Scan(&id)
	if err != nil {
		return fmt.Errorf("postgres.NoteSyncStore.CreateTarget: %w", translateError(err))
	}
	target.ID = id
	created, err := s.GetTarget(target.UserID, id)
	if err != nil {
		return fmt.Errorf("postgres.NoteSyncStore.CreateTarget readback: %w", err)
	}
	*target = *created
	return nil
}

func (s *NoteSyncStore) GetTarget(userID, targetID int64) (*notesync.SyncTarget, error) {
	row := s.db.QueryRowContext(context.Background(), `SELECT `+syncTargetColumns+` FROM note_sync_targets WHERE id = $1 AND user_id = $2`, targetID, userID)
	target, err := scanSyncTarget(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notesync.ErrTargetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteSyncStore.GetTarget: %w", translateError(err))
	}
	return target, nil
}

func (s *NoteSyncStore) ListTargets(userID int64) ([]notesync.SyncTarget, error) {
	rows, err := s.db.QueryContext(context.Background(), `SELECT `+syncTargetColumns+` FROM note_sync_targets WHERE user_id = $1 ORDER BY updated_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteSyncStore.ListTargets: %w", translateError(err))
	}
	defer rows.Close()
	result := make([]notesync.SyncTarget, 0)
	for rows.Next() {
		item, scanErr := scanSyncTarget(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres.NoteSyncStore.ListTargets scan: %w", scanErr)
		}
		result = append(result, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.NoteSyncStore.ListTargets rows: %w", err)
	}
	return result, nil
}

func (s *NoteSyncStore) UpdateTarget(target *notesync.SyncTarget) error {
	tags := normalizeStringSlice(target.SyncTags)
	result, err := s.db.ExecContext(context.Background(), `UPDATE note_sync_targets SET name = $1, data_source_id = $2,
        token_env = $3, title_property = $4, tags_property = $5, flow_id_property = $6, sync_tags = $7::text[], enabled = $8, auto_push = $9, updated_at = $10
        WHERE id = $11 AND user_id = $12`, target.Name, target.DataSourceID, target.TokenEnv, target.TitleProperty, target.TagsProperty, target.FlowIDProperty, tags, target.Enabled, target.AutoPush, s.now().UTC(), target.ID, target.UserID)
	if err != nil {
		return fmt.Errorf("postgres.NoteSyncStore.UpdateTarget: %w", translateError(err))
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return notesync.ErrTargetNotFound
	}
	updated, err := s.GetTarget(target.UserID, target.ID)
	if err != nil {
		return err
	}
	*target = *updated
	return nil
}

func (s *NoteSyncStore) DeleteTarget(userID, targetID int64) error {
	result, err := s.db.ExecContext(context.Background(), `DELETE FROM note_sync_targets WHERE id = $1 AND user_id = $2`, targetID, userID)
	if err != nil {
		return fmt.Errorf("postgres.NoteSyncStore.DeleteTarget: %w", translateError(err))
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return notesync.ErrTargetNotFound
	}
	return nil
}

func (s *NoteSyncStore) CountBindings(userID, targetID int64) (int, error) {
	count, err := countRows(context.Background(), s.db, `SELECT COUNT(*) FROM note_sync_bindings b JOIN notes n ON n.id = b.note_id JOIN note_sync_targets t ON t.id = b.target_id WHERE b.target_id = $1 AND n.user_id = $2 AND t.user_id = $2`, targetID, userID)
	if err != nil {
		return 0, fmt.Errorf("postgres.NoteSyncStore.CountBindings: %w", err)
	}
	return count, nil
}

func (s *NoteSyncStore) GetBinding(userID, noteID int64) (*notesync.SyncBinding, error) {
	row := s.db.QueryRowContext(context.Background(), `SELECT b.note_id, b.target_id, b.created_at, b.updated_at FROM note_sync_bindings b JOIN notes n ON n.id = b.note_id JOIN note_sync_targets t ON t.id = b.target_id WHERE b.note_id = $1 AND n.user_id = $2 AND t.user_id = $2`, noteID, userID)
	binding, err := scanSyncBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notesync.ErrBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteSyncStore.GetBinding: %w", translateError(err))
	}
	return binding, nil
}

func (s *NoteSyncStore) SetBinding(userID, noteID, targetID int64) error {
	var exists bool
	if err := s.db.QueryRowContext(context.Background(), `SELECT EXISTS(SELECT 1 FROM notes n JOIN note_sync_targets t ON t.user_id = n.user_id WHERE n.id = $1 AND n.user_id = $2 AND t.id = $3 AND t.user_id = $2)`, noteID, userID, targetID).Scan(&exists); err != nil {
		return fmt.Errorf("postgres.NoteSyncStore.SetBinding validate: %w", translateError(err))
	}
	if !exists {
		return notesync.ErrTargetNotFound
	}
	_, err := s.db.ExecContext(context.Background(), `INSERT INTO note_sync_bindings (note_id, target_id) VALUES ($1, $2) ON CONFLICT (note_id) DO UPDATE SET target_id = EXCLUDED.target_id, updated_at = $3`, noteID, targetID, s.now().UTC())
	if err != nil {
		return fmt.Errorf("postgres.NoteSyncStore.SetBinding: %w", translateError(err))
	}
	return nil
}

func (s *NoteSyncStore) ClearBinding(userID, noteID int64) error {
	result, err := s.db.ExecContext(context.Background(), `DELETE FROM note_sync_bindings b USING notes n WHERE b.note_id = n.id AND b.note_id = $1 AND n.user_id = $2`, noteID, userID)
	if err != nil {
		return fmt.Errorf("postgres.NoteSyncStore.ClearBinding: %w", translateError(err))
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return notesync.ErrBindingNotFound
	}
	return nil
}

func (s *NoteSyncStore) GetState(userID, noteID, targetID int64) (*notesync.SyncState, error) {
	row := s.db.QueryRowContext(context.Background(), `SELECT `+syncStateColumns+` FROM note_sync_states st JOIN notes n ON n.id = st.note_id JOIN note_sync_targets t ON t.id = st.target_id WHERE st.note_id = $1 AND st.target_id = $2 AND n.user_id = $3 AND t.user_id = $3`, noteID, targetID, userID)
	state, err := scanSyncState(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notesync.ErrStateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteSyncStore.GetState: %w", translateError(err))
	}
	return state, nil
}

func (s *NoteSyncStore) GetStateByPage(userID, targetID int64, pageID string) (*notesync.SyncState, error) {
	row := s.db.QueryRowContext(context.Background(), `SELECT `+syncStateColumns+` FROM note_sync_states st JOIN notes n ON n.id = st.note_id JOIN note_sync_targets t ON t.id = st.target_id WHERE st.target_id = $1 AND st.external_page_id = $2 AND n.user_id = $3 AND t.user_id = $3`, targetID, pageID, userID)
	state, err := scanSyncState(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notesync.ErrStateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteSyncStore.GetStateByPage: %w", translateError(err))
	}
	return state, nil
}

func (s *NoteSyncStore) SaveState(state *notesync.SyncState) error {
	_, err := s.db.ExecContext(context.Background(), `INSERT INTO note_sync_states
        (note_id, target_id, external_page_id, external_url, local_hash, external_hash, last_direction, last_synced_at, last_error, pending_delete, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, COALESCE($11, now()))
        ON CONFLICT (note_id, target_id) DO UPDATE SET external_page_id = EXCLUDED.external_page_id, external_url = EXCLUDED.external_url,
        local_hash = EXCLUDED.local_hash, external_hash = EXCLUDED.external_hash, last_direction = EXCLUDED.last_direction,
        last_synced_at = EXCLUDED.last_synced_at, last_error = EXCLUDED.last_error, pending_delete = EXCLUDED.pending_delete, updated_at = EXCLUDED.updated_at`,
		state.NoteID, state.TargetID, state.ExternalPageID, state.ExternalURL, state.LocalHash, state.ExternalHash, state.LastDirection, nullableTime(state.LastSyncedAt), state.LastError, state.PendingDelete, nullableTime(&state.UpdatedAt))
	if err != nil {
		return fmt.Errorf("postgres.NoteSyncStore.SaveState: %w", translateError(err))
	}
	return nil
}

func (s *NoteSyncStore) ListBoundNoteIDs(userID, targetID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(context.Background(), `SELECT b.note_id FROM note_sync_bindings b JOIN notes n ON n.id = b.note_id JOIN note_sync_targets t ON t.id = b.target_id WHERE b.target_id = $1 AND n.user_id = $2 AND t.user_id = $2 ORDER BY b.note_id`, targetID, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteSyncStore.ListBoundNoteIDs: %w", translateError(err))
	}
	defer rows.Close()
	var result []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("postgres.NoteSyncStore.ListBoundNoteIDs scan: %w", err)
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (s *NoteSyncStore) ListStates(userID, targetID int64) ([]notesync.SyncState, error) {
	rows, err := s.db.QueryContext(context.Background(), `SELECT `+syncStateColumns+` FROM note_sync_states st JOIN notes n ON n.id = st.note_id JOIN note_sync_targets t ON t.id = st.target_id WHERE st.target_id = $1 AND n.user_id = $2 AND t.user_id = $2 ORDER BY st.updated_at DESC`, targetID, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres.NoteSyncStore.ListStates: %w", translateError(err))
	}
	defer rows.Close()
	var result []notesync.SyncState
	for rows.Next() {
		state, scanErr := scanSyncState(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres.NoteSyncStore.ListStates scan: %w", scanErr)
		}
		result = append(result, *state)
	}
	return result, rows.Err()
}

func scanSyncTarget(scanner interface{ Scan(...any) error }) (*notesync.SyncTarget, error) {
	var target notesync.SyncTarget
	var tagsJSON string
	if err := scanner.Scan(&target.ID, &target.UserID, &target.Type, &target.Name, &target.DataSourceID, &target.TokenEnv, &target.TitleProperty, &target.TagsProperty, &target.FlowIDProperty, &tagsJSON, &target.Enabled, &target.AutoPush, &target.CreatedAt, &target.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(tagsJSON), &target.SyncTags); err != nil {
		return nil, fmt.Errorf("scanSyncTarget tags: %w", err)
	}
	return &target, nil
}

func scanSyncBinding(scanner interface{ Scan(...any) error }) (*notesync.SyncBinding, error) {
	var binding notesync.SyncBinding
	return &binding, scanner.Scan(&binding.NoteID, &binding.TargetID, &binding.CreatedAt, &binding.UpdatedAt)
}

func scanSyncState(scanner interface{ Scan(...any) error }) (*notesync.SyncState, error) {
	var state notesync.SyncState
	var lastSynced sql.NullTime
	var lastDirection sql.NullString
	if err := scanner.Scan(&state.NoteID, &state.TargetID, &state.ExternalPageID, &state.ExternalURL, &state.LocalHash, &state.ExternalHash, &lastDirection, &lastSynced, &state.LastError, &state.PendingDelete, &state.UpdatedAt); err != nil {
		return nil, err
	}
	if lastDirection.Valid {
		state.LastDirection = lastDirection.String
	}
	if lastSynced.Valid {
		state.LastSyncedAt = &lastSynced.Time
	}
	return &state, nil
}
