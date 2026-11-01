package data

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/notesync"
)

// NoteSyncStore persists Notion targets, bindings and external page state in SQLite.
type NoteSyncStore struct {
	db *sql.DB
}

func NewNoteSyncStore(db *sql.DB) *NoteSyncStore { return &NoteSyncStore{db: db} }

func (s *NoteSyncStore) CreateTarget(target *notesync.SyncTarget) error {
	tags, err := json.Marshal(target.SyncTags)
	if err != nil {
		return fmt.Errorf("NoteSyncStore.CreateTarget marshal tags: %w", err)
	}
	result, err := s.db.Exec(`INSERT INTO note_sync_targets
        (user_id, type, name, data_source_id, token_env, title_property, tags_property, flow_id_property, sync_tags_json, enabled, auto_push)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		target.UserID, target.Type, target.Name, target.DataSourceID, target.TokenEnv, target.TitleProperty, target.TagsProperty, target.FlowIDProperty, string(tags), boolInt(target.Enabled), boolInt(target.AutoPush))
	if err != nil {
		slog.Error("NoteSyncStore.CreateTarget failed", "err", err, "user_id", target.UserID)
		return fmt.Errorf("NoteSyncStore.CreateTarget: %w", err)
	}
	target.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("NoteSyncStore.CreateTarget last insert id: %w", err)
	}
	created, err := s.GetTarget(target.UserID, target.ID)
	if err != nil {
		return fmt.Errorf("NoteSyncStore.CreateTarget readback: %w", err)
	}
	*target = *created
	return nil
}

func (s *NoteSyncStore) GetTarget(userID, targetID int64) (*notesync.SyncTarget, error) {
	row := s.db.QueryRow(`SELECT id, user_id, type, name, data_source_id, token_env,
        title_property, tags_property, flow_id_property, sync_tags_json, enabled, auto_push, created_at, updated_at
        FROM note_sync_targets WHERE id = ? AND user_id = ?`, targetID, userID)
	target, err := scanSyncTarget(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notesync.ErrTargetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("NoteSyncStore.GetTarget: %w", err)
	}
	return target, nil
}

func (s *NoteSyncStore) ListTargets(userID int64) ([]notesync.SyncTarget, error) {
	rows, err := s.db.Query(`SELECT id, user_id, type, name, data_source_id, token_env,
        title_property, tags_property, flow_id_property, sync_tags_json, enabled, auto_push, created_at, updated_at
        FROM note_sync_targets WHERE user_id = ? ORDER BY updated_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("NoteSyncStore.ListTargets query: %w", err)
	}
	defer rows.Close()
	result := make([]notesync.SyncTarget, 0)
	for rows.Next() {
		target, scanErr := scanSyncTarget(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("NoteSyncStore.ListTargets scan: %w", scanErr)
		}
		result = append(result, *target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("NoteSyncStore.ListTargets rows: %w", err)
	}
	return result, nil
}

func (s *NoteSyncStore) UpdateTarget(target *notesync.SyncTarget) error {
	tags, err := json.Marshal(target.SyncTags)
	if err != nil {
		return fmt.Errorf("NoteSyncStore.UpdateTarget marshal tags: %w", err)
	}
	result, err := s.db.Exec(`UPDATE note_sync_targets SET name = ?, data_source_id = ?, token_env = ?,
        title_property = ?, tags_property = ?, flow_id_property = ?, sync_tags_json = ?, enabled = ?, auto_push = ?, updated_at = datetime('now')
        WHERE id = ? AND user_id = ?`, target.Name, target.DataSourceID, target.TokenEnv, target.TitleProperty, target.TagsProperty,
		target.FlowIDProperty, string(tags), boolInt(target.Enabled), boolInt(target.AutoPush), target.ID, target.UserID)
	if err != nil {
		return fmt.Errorf("NoteSyncStore.UpdateTarget: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return notesync.ErrTargetNotFound
	}
	updated, err := s.GetTarget(target.UserID, target.ID)
	if err != nil {
		return fmt.Errorf("NoteSyncStore.UpdateTarget readback: %w", err)
	}
	*target = *updated
	return nil
}

func (s *NoteSyncStore) DeleteTarget(userID, targetID int64) error {
	result, err := s.db.Exec(`DELETE FROM note_sync_targets WHERE id = ? AND user_id = ?`, targetID, userID)
	if err != nil {
		return fmt.Errorf("NoteSyncStore.DeleteTarget: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return notesync.ErrTargetNotFound
	}
	return nil
}

func (s *NoteSyncStore) CountBindings(userID, targetID int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM note_sync_bindings b
        JOIN notes n ON n.id = b.note_id
        JOIN note_sync_targets t ON t.id = b.target_id
        WHERE n.user_id = ? AND t.user_id = ? AND t.id = ?`, userID, userID, targetID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("NoteSyncStore.CountBindings: %w", err)
	}
	return count, nil
}

func (s *NoteSyncStore) GetBinding(userID, noteID int64) (*notesync.SyncBinding, error) {
	row := s.db.QueryRow(`SELECT b.note_id, b.target_id, b.created_at, b.updated_at
        FROM note_sync_bindings b JOIN notes n ON n.id = b.note_id JOIN note_sync_targets t ON t.id = b.target_id
        WHERE b.note_id = ? AND n.user_id = ? AND t.user_id = ?`, noteID, userID, userID)
	binding, err := scanSyncBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notesync.ErrBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("NoteSyncStore.GetBinding: %w", err)
	}
	return binding, nil
}

func (s *NoteSyncStore) SetBinding(userID, noteID, targetID int64) error {
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM notes n JOIN note_sync_targets t ON t.user_id = n.user_id
        WHERE n.id = ? AND n.user_id = ? AND t.id = ? AND t.user_id = ?`, noteID, userID, targetID, userID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notesync.ErrTargetNotFound
		}
		return fmt.Errorf("NoteSyncStore.SetBinding validate: %w", err)
	}
	_, err := s.db.Exec(`INSERT INTO note_sync_bindings (note_id, target_id) VALUES (?, ?)
        ON CONFLICT(note_id) DO UPDATE SET target_id = excluded.target_id, updated_at = datetime('now')`, noteID, targetID)
	if err != nil {
		return fmt.Errorf("NoteSyncStore.SetBinding: %w", err)
	}
	return nil
}

func (s *NoteSyncStore) ClearBinding(userID, noteID int64) error {
	result, err := s.db.Exec(`DELETE FROM note_sync_bindings WHERE note_id = ? AND note_id IN (SELECT id FROM notes WHERE id = ? AND user_id = ?)`, noteID, noteID, userID)
	if err != nil {
		return fmt.Errorf("NoteSyncStore.ClearBinding: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return notesync.ErrBindingNotFound
	}
	return nil
}

func (s *NoteSyncStore) GetState(userID, noteID, targetID int64) (*notesync.SyncState, error) {
	row := s.db.QueryRow(`SELECT st.note_id, st.target_id, st.external_page_id, st.external_url, st.local_hash,
        st.external_hash, st.last_direction, st.last_synced_at, st.last_error, st.pending_delete, st.updated_at
        FROM note_sync_states st JOIN notes n ON n.id = st.note_id JOIN note_sync_targets t ON t.id = st.target_id
        WHERE st.note_id = ? AND st.target_id = ? AND n.user_id = ? AND t.user_id = ?`, noteID, targetID, userID, userID)
	state, err := scanSyncState(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notesync.ErrStateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("NoteSyncStore.GetState: %w", err)
	}
	return state, nil
}

func (s *NoteSyncStore) GetStateByPage(userID, targetID int64, pageID string) (*notesync.SyncState, error) {
	row := s.db.QueryRow(`SELECT st.note_id, st.target_id, st.external_page_id, st.external_url, st.local_hash,
        st.external_hash, st.last_direction, st.last_synced_at, st.last_error, st.pending_delete, st.updated_at
        FROM note_sync_states st JOIN notes n ON n.id = st.note_id JOIN note_sync_targets t ON t.id = st.target_id
        WHERE st.target_id = ? AND st.external_page_id = ? AND n.user_id = ? AND t.user_id = ?`, targetID, pageID, userID, userID)
	state, err := scanSyncState(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notesync.ErrStateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("NoteSyncStore.GetStateByPage: %w", err)
	}
	return state, nil
}

func (s *NoteSyncStore) SaveState(state *notesync.SyncState) error {
	_, err := s.db.Exec(`INSERT INTO note_sync_states
        (note_id, target_id, external_page_id, external_url, local_hash, external_hash, last_direction, last_synced_at, last_error, pending_delete, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, COALESCE(?, datetime('now')))
        ON CONFLICT(note_id, target_id) DO UPDATE SET external_page_id = excluded.external_page_id,
        external_url = excluded.external_url, local_hash = excluded.local_hash, external_hash = excluded.external_hash,
        last_direction = excluded.last_direction, last_synced_at = excluded.last_synced_at, last_error = excluded.last_error,
        pending_delete = excluded.pending_delete, updated_at = excluded.updated_at`, state.NoteID, state.TargetID, state.ExternalPageID, state.ExternalURL,
		state.LocalHash, state.ExternalHash, state.LastDirection, formatSQLiteTimePtr(state.LastSyncedAt), state.LastError, boolInt(state.PendingDelete), formatSQLiteTime(state.UpdatedAt))
	if err != nil {
		return fmt.Errorf("NoteSyncStore.SaveState: %w", err)
	}
	return nil
}

func (s *NoteSyncStore) ListBoundNoteIDs(userID, targetID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT b.note_id FROM note_sync_bindings b JOIN notes n ON n.id = b.note_id JOIN note_sync_targets t ON t.id = b.target_id WHERE b.target_id = ? AND n.user_id = ? AND t.user_id = ? ORDER BY b.note_id`, targetID, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("NoteSyncStore.ListBoundNoteIDs: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("NoteSyncStore.ListBoundNoteIDs scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *NoteSyncStore) ListStates(userID, targetID int64) ([]notesync.SyncState, error) {
	rows, err := s.db.Query(`SELECT st.note_id, st.target_id, st.external_page_id, st.external_url, st.local_hash,
        st.external_hash, st.last_direction, st.last_synced_at, st.last_error, st.pending_delete, st.updated_at
        FROM note_sync_states st JOIN notes n ON n.id = st.note_id JOIN note_sync_targets t ON t.id = st.target_id
        WHERE st.target_id = ? AND n.user_id = ? AND t.user_id = ? ORDER BY st.updated_at DESC`, targetID, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("NoteSyncStore.ListStates query: %w", err)
	}
	defer rows.Close()
	var result []notesync.SyncState
	for rows.Next() {
		state, scanErr := scanSyncState(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("NoteSyncStore.ListStates scan: %w", scanErr)
		}
		result = append(result, *state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("NoteSyncStore.ListStates rows: %w", err)
	}
	return result, nil
}

func scanSyncTarget(scanner interface{ Scan(...any) error }) (*notesync.SyncTarget, error) {
	var target notesync.SyncTarget
	var tagsJSON, createdAt, updatedAt string
	var enabled, autoPush int
	err := scanner.Scan(&target.ID, &target.UserID, &target.Type, &target.Name, &target.DataSourceID, &target.TokenEnv, &target.TitleProperty, &target.TagsProperty, &target.FlowIDProperty, &tagsJSON, &enabled, &autoPush, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(tagsJSON), &target.SyncTags); err != nil {
		return nil, fmt.Errorf("scanSyncTarget tags: %w", err)
	}
	target.Enabled, target.AutoPush = enabled != 0, autoPush != 0
	parsedCreated, parseErr := parseSQLiteTime(createdAt)
	if parseErr != nil {
		return nil, fmt.Errorf("scanSyncTarget created_at: %w", parseErr)
	}
	parsedUpdated, parseErr := parseSQLiteTime(updatedAt)
	if parseErr != nil {
		return nil, fmt.Errorf("scanSyncTarget updated_at: %w", parseErr)
	}
	target.CreatedAt, target.UpdatedAt = parsedCreated, parsedUpdated
	return &target, nil
}

func scanSyncBinding(scanner interface{ Scan(...any) error }) (*notesync.SyncBinding, error) {
	var binding notesync.SyncBinding
	var createdAt, updatedAt string
	if err := scanner.Scan(&binding.NoteID, &binding.TargetID, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var err error
	binding.CreatedAt, err = parseSQLiteTime(createdAt)
	if err != nil {
		return nil, err
	}
	binding.UpdatedAt, err = parseSQLiteTime(updatedAt)
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

func scanSyncState(scanner interface{ Scan(...any) error }) (*notesync.SyncState, error) {
	var state notesync.SyncState
	var lastSynced sql.NullString
	var updated string
	var lastDirection sql.NullString
	var pending int
	if err := scanner.Scan(&state.NoteID, &state.TargetID, &state.ExternalPageID, &state.ExternalURL, &state.LocalHash, &state.ExternalHash, &lastDirection, &lastSynced, &state.LastError, &pending, &updated); err != nil {
		return nil, err
	}
	if lastDirection.Valid {
		state.LastDirection = lastDirection.String
	}
	state.PendingDelete = pending != 0
	if lastSynced.Valid && lastSynced.String != "" {
		parsed, err := parseSQLiteTime(lastSynced.String)
		if err != nil {
			return nil, fmt.Errorf("scanSyncState last_synced_at: %w", err)
		}
		state.LastSyncedAt = &parsed
	}
	parsed, err := parseSQLiteTime(updated)
	if err != nil {
		return nil, fmt.Errorf("scanSyncState updated_at: %w", err)
	}
	state.UpdatedAt = parsed
	return &state, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
