package notesync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"japanese-learning-app/internal/integration/notion"
	"japanese-learning-app/internal/module/note"
)

// NewEnvClientFactory creates a Notion client factory that resolves tokens from
// approved environment variables at request time.
func NewEnvClientFactory(baseURL string) ClientFactory {
	return func(target SyncTarget) (NotionClient, error) {
		cfg := notion.Config{
			DataSourceID:   target.DataSourceID,
			TokenEnv:       target.TokenEnv,
			TitleProperty:  target.TitleProperty,
			TagsProperty:   target.TagsProperty,
			FlowIDProperty: target.FlowIDProperty,
			SyncTags:       target.SyncTags,
		}
		token, err := notion.ResolveToken(cfg, os.Getenv)
		if err != nil {
			return nil, fmt.Errorf("notesync resolve token: %w", err)
		}
		client, err := notion.NewClientChecked(token, baseURL)
		if err != nil {
			return nil, fmt.Errorf("notesync create notion client: %w", err)
		}
		return clientAdapter{client}, nil
	}
}

type clientAdapter struct{ client *notion.Client }

func (c clientAdapter) TestDataSource(id string) error { return c.client.TestDataSource(id) }
func (c clientAdapter) QueryDataSource(id string) ([]notion.Page, error) {
	return c.client.QueryDataSource(id)
}
func (c clientAdapter) RetrievePageBlocks(id string) ([]notion.Block, error) {
	return c.client.RetrievePageBlocks(id)
}
func (c clientAdapter) CreatePage(input notion.PageInput) (notion.Page, error) {
	return c.client.CreatePage(input)
}
func (c clientAdapter) UpdatePage(id string, input notion.PageInput) (notion.Page, error) {
	return c.client.UpdatePage(id, input)
}
func (c clientAdapter) ReplacePageBlocks(id string, blocks []notion.Block) error {
	return c.client.ReplacePageBlocks(id, blocks)
}
func (c clientAdapter) RestorePage(id string) error { return c.client.RestorePage(id) }

func (s *Service) CreateTarget(userID int64, req TargetRequest) (*SyncTarget, error) {
	target, err := targetFromRequest(userID, 0, req)
	if err != nil {
		return nil, fmt.Errorf("notesync create target validate: %w", err)
	}
	if err := s.store.CreateTarget(target); err != nil {
		slog.Error("notesync create target failed", "user_id", userID, "err", err)
		return nil, fmt.Errorf("notesync create target: %w", err)
	}
	target.HasToken = s.hasToken(*target)
	return target, nil
}

func (s *Service) UpdateTarget(userID, targetID int64, req TargetRequest) (*SyncTarget, error) {
	existing, err := s.store.GetTarget(userID, targetID)
	if err != nil {
		return nil, fmt.Errorf("notesync update target load: %w", err)
	}
	updated, err := targetFromRequest(userID, targetID, req)
	if err != nil {
		return nil, fmt.Errorf("notesync update target validate: %w", err)
	}
	if updated.DataSourceID != existing.DataSourceID {
		count, countErr := s.store.CountBindings(userID, targetID)
		if countErr != nil {
			return nil, fmt.Errorf("notesync update target count bindings: %w", countErr)
		}
		if count > 0 {
			return nil, ErrIdentityLocked
		}
	}
	updated.CreatedAt = existing.CreatedAt
	if err := s.store.UpdateTarget(updated); err != nil {
		return nil, fmt.Errorf("notesync update target: %w", err)
	}
	updated.HasToken = s.hasToken(*updated)
	return updated, nil
}

func (s *Service) ListTargets(userID int64) ([]SyncTarget, error) {
	targets, err := s.store.ListTargets(userID)
	if err != nil {
		return nil, fmt.Errorf("notesync list targets: %w", err)
	}
	for i := range targets {
		targets[i].HasToken = s.hasToken(targets[i])
	}
	return targets, nil
}

func (s *Service) GetTarget(userID, targetID int64) (*SyncTarget, error) {
	target, err := s.store.GetTarget(userID, targetID)
	if err != nil {
		return nil, fmt.Errorf("notesync get target: %w", err)
	}
	target.HasToken = s.hasToken(*target)
	return target, nil
}

func (s *Service) DeleteTarget(userID, targetID int64) error {
	count, err := s.store.CountBindings(userID, targetID)
	if err != nil {
		return fmt.Errorf("notesync delete target count bindings: %w", err)
	}
	if count > 0 {
		return ErrTargetInUse
	}
	if err := s.store.DeleteTarget(userID, targetID); err != nil {
		return fmt.Errorf("notesync delete target: %w", err)
	}
	return nil
}

func (s *Service) TestTarget(userID int64, req TargetRequest) error {
	target, err := targetFromRequest(userID, 0, req)
	if err != nil {
		return fmt.Errorf("notesync test target validate: %w", err)
	}
	client, err := s.clientFor(*target)
	if err != nil {
		return err
	}
	if err := client.TestDataSource(target.DataSourceID); err != nil {
		return fmt.Errorf("notesync test target: %w", err)
	}
	return nil
}

func (s *Service) GetBinding(userID, noteID int64) (*SyncBinding, error) {
	binding, err := s.store.GetBinding(userID, noteID)
	if err != nil {
		if errors.Is(err, ErrBindingNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("notesync get binding: %w", err)
	}
	return binding, nil
}

func (s *Service) BindNote(userID, noteID, targetID int64) error {
	if _, err := s.notes.GetByID(userID, noteID); err != nil {
		return fmt.Errorf("notesync bind note load note: %w", err)
	}
	target, err := s.store.GetTarget(userID, targetID)
	if err != nil {
		return fmt.Errorf("notesync bind note load target: %w", err)
	}
	if !target.Enabled {
		return errors.New("notion sync target is disabled")
	}
	if err := s.store.SetBinding(userID, noteID, targetID); err != nil {
		return fmt.Errorf("notesync bind note: %w", err)
	}
	return nil
}

func (s *Service) ClearBinding(userID, noteID int64) error {
	if _, err := s.notes.GetByID(userID, noteID); err != nil {
		return fmt.Errorf("notesync clear binding load note: %w", err)
	}
	if err := s.store.ClearBinding(userID, noteID); err != nil {
		if errors.Is(err, ErrBindingNotFound) {
			// DELETE is intentionally idempotent so the UI can safely clear an
			// already-cleared selection.
			return nil
		}
		return fmt.Errorf("notesync clear binding: %w", err)
	}
	return nil
}

func (s *Service) GetState(userID, noteID int64) (*SyncState, error) {
	binding, err := s.store.GetBinding(userID, noteID)
	if err != nil {
		if errors.Is(err, ErrBindingNotFound) {
			return nil, nil
		}
		return nil, err
	}
	state, err := s.store.GetState(userID, noteID, binding.TargetID)
	if errors.Is(err, ErrStateNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("notesync get state: %w", err)
	}
	return state, nil
}

func (s *Service) PushNote(userID, noteID int64) (*SyncState, error) {
	binding, err := s.store.GetBinding(userID, noteID)
	if err != nil {
		return nil, fmt.Errorf("notesync push load binding: %w", err)
	}
	target, err := s.store.GetTarget(userID, binding.TargetID)
	if err != nil {
		return nil, fmt.Errorf("notesync push load target: %w", err)
	}
	if !target.Enabled {
		return nil, errors.New("notion sync target is disabled")
	}
	state, stateErr := s.store.GetState(userID, noteID, binding.TargetID)
	if stateErr != nil && !errors.Is(stateErr, ErrStateNotFound) {
		return nil, fmt.Errorf("notesync push load state: %w", stateErr)
	}
	n, err := s.notes.GetByID(userID, noteID)
	if err != nil {
		return nil, fmt.Errorf("notesync push load note: %w", err)
	}
	client, err := s.clientFor(*target)
	if err != nil {
		s.rememberStateError(state, err)
		return nil, err
	}
	blocks, err := notion.MarkdownToBlocks(n.Content, n.SourceText)
	if err != nil {
		s.rememberStateError(state, err)
		return nil, fmt.Errorf("notesync push blocks: %w", err)
	}
	input := pageInput(*target, *n, blocks)
	var page notion.Page
	if state == nil || strings.TrimSpace(state.ExternalPageID) == "" {
		page, err = client.CreatePage(input)
	} else {
		page, err = client.UpdatePage(state.ExternalPageID, input)
		if err == nil {
			err = client.ReplacePageBlocks(state.ExternalPageID, blocks)
		}
	}
	if err != nil {
		slog.Error("notesync push failed", "user_id", userID, "note_id", noteID, "target_id", target.ID, "err", err)
		s.rememberStateError(state, err)
		return nil, fmt.Errorf("notesync push page: %w", err)
	}
	if strings.TrimSpace(page.ID) == "" {
		err := errors.New("notesync push: Notion returned an empty page id")
		s.rememberStateError(state, err)
		return nil, err
	}
	now := s.now()
	newState := &SyncState{
		NoteID:         noteID,
		TargetID:       target.ID,
		ExternalPageID: page.ID,
		ExternalURL:    page.URL,
		LocalHash:      noteHash(*n),
		ExternalHash:   noteHash(*n),
		LastDirection:  "push",
		LastSyncedAt:   &now,
		UpdatedAt:      now,
	}
	if err := s.store.SaveState(newState); err != nil {
		return nil, fmt.Errorf("notesync push save state: %w", err)
	}
	return newState, nil
}

func (s *Service) rememberStateError(state *SyncState, cause error) {
	if state == nil || cause == nil {
		return
	}
	state.LastError = cause.Error()
	state.UpdatedAt = s.now()
	if err := s.store.SaveState(state); err != nil {
		slog.Error("notesync save sync error failed", "note_id", state.NoteID, "target_id", state.TargetID, "err", err)
	}
}

// PushTarget pushes every note currently bound to a target. Each note is
// isolated so one failing page does not abort the remaining batch.
func (s *Service) PushTarget(userID, targetID int64) (SyncResult, error) {
	var result SyncResult
	if _, err := s.store.GetTarget(userID, targetID); err != nil {
		return result, fmt.Errorf("notesync push target load target: %w", err)
	}
	noteIDs, err := s.store.ListBoundNoteIDs(userID, targetID)
	if err != nil {
		return result, fmt.Errorf("notesync push target list notes: %w", err)
	}
	for _, noteID := range noteIDs {
		before, stateErr := s.store.GetState(userID, noteID, targetID)
		if stateErr != nil && !errors.Is(stateErr, ErrStateNotFound) {
			result.Errors = append(result.Errors, SyncItemError{NoteID: noteID, Message: stateErr.Error()})
			continue
		}
		if _, pushErr := s.PushNote(userID, noteID); pushErr != nil {
			result.Errors = append(result.Errors, SyncItemError{NoteID: noteID, Message: pushErr.Error()})
			continue
		}
		if before == nil || strings.TrimSpace(before.ExternalPageID) == "" {
			result.Created++
		} else {
			result.Updated++
		}
	}
	return result, nil
}

func (s *Service) PullTarget(userID, targetID int64) (SyncResult, error) {
	var result SyncResult
	target, err := s.store.GetTarget(userID, targetID)
	if err != nil {
		return result, fmt.Errorf("notesync pull load target: %w", err)
	}
	if !target.Enabled {
		return result, errors.New("notion sync target is disabled")
	}
	client, err := s.clientFor(*target)
	if err != nil {
		return result, err
	}
	pages, err := client.QueryDataSource(target.DataSourceID)
	if err != nil {
		return result, fmt.Errorf("notesync pull query data source: %w", err)
	}
	seen := make(map[string]struct{}, len(pages))
	for _, page := range pages {
		if strings.TrimSpace(page.ID) == "" {
			result.Skipped++
			continue
		}
		seen[page.ID] = struct{}{}
		external := ExternalPage{ID: page.ID, URL: page.URL, LastEditedTime: page.LastEditedTime, Archived: page.Archived, InTrash: page.InTrash, Title: pageTitle(page, target.TitleProperty), Tags: pageTags(page, target.TagsProperty), FlowID: pageTextProperty(page, target.FlowIDProperty)}
		if !matchesSyncTags(external.Tags, target.SyncTags) {
			result.Skipped++
			continue
		}
		blocks, blockErr := client.RetrievePageBlocks(page.ID)
		if blockErr != nil {
			result.Errors = append(result.Errors, SyncItemError{PageID: page.ID, Message: blockErr.Error()})
			continue
		}
		content, sourceText, convertErr := parsePageBlocks(blocks)
		if convertErr != nil {
			result.Errors = append(result.Errors, SyncItemError{PageID: page.ID, Message: convertErr.Error()})
			continue
		}
		external.Content, external.SourceText = content, sourceText
		state, stateErr := s.store.GetStateByPage(userID, targetID, page.ID)
		if stateErr != nil && !errors.Is(stateErr, ErrStateNotFound) {
			result.Errors = append(result.Errors, SyncItemError{PageID: page.ID, Message: stateErr.Error()})
			continue
		}
		stateWasKnown := state != nil
		var local *note.Note
		if state != nil {
			local, _ = s.notes.GetByID(userID, state.NoteID)
		}
		if local == nil && external.FlowID != "" {
			if parsed, parseErr := parseNoteID(external.FlowID); parseErr == nil {
				local, _ = s.notes.GetByID(userID, parsed)
			}
		}
		if local == nil {
			if strings.TrimSpace(external.Title) == "" {
				result.Skipped++
				continue
			}
			local = &note.Note{UserID: userID, Type: note.TypeSentence, Title: external.Title, Content: external.Content, SourceText: external.SourceText, Tags: external.Tags}
			if err := s.notes.Create(local); err != nil {
				result.Errors = append(result.Errors, SyncItemError{PageID: page.ID, Message: err.Error()})
				continue
			}
			result.Imported++
		} else {
			local.Title, local.Content, local.SourceText, local.Tags = external.Title, external.Content, external.SourceText, external.Tags
			if err := s.notes.Update(local); err != nil {
				result.Errors = append(result.Errors, SyncItemError{NoteID: local.ID, PageID: page.ID, Message: err.Error()})
				continue
			}
			result.Updated++
		}
		if !stateWasKnown {
			if err := s.store.SetBinding(userID, local.ID, targetID); err != nil {
				result.Errors = append(result.Errors, SyncItemError{NoteID: local.ID, PageID: page.ID, Message: fmt.Errorf("bind imported note: %w", err).Error()})
				continue
			}
		}
		now := s.now()
		newState := &SyncState{NoteID: local.ID, TargetID: targetID, ExternalPageID: page.ID, ExternalURL: page.URL, LocalHash: noteHash(*local), ExternalHash: externalHash(external), LastDirection: "pull", LastSyncedAt: &now, UpdatedAt: now}
		if err := s.store.SaveState(newState); err != nil {
			result.Errors = append(result.Errors, SyncItemError{NoteID: local.ID, PageID: page.ID, Message: err.Error()})
		}
	}
	states, err := s.store.ListStates(userID, targetID)
	if err != nil {
		return result, fmt.Errorf("notesync pull list states: %w", err)
	}
	for _, state := range states {
		if state.ExternalPageID == "" || state.PendingDelete {
			continue
		}
		if _, ok := seen[state.ExternalPageID]; ok {
			continue
		}
		state.PendingDelete = true
		state.UpdatedAt = s.now()
		if err := s.store.SaveState(&state); err != nil {
			result.Errors = append(result.Errors, SyncItemError{NoteID: state.NoteID, PageID: state.ExternalPageID, Message: err.Error()})
			continue
		}
		result.PendingDelete++
	}
	return result, nil
}

func (s *Service) ListPendingDeletes(userID, targetID int64) ([]SyncState, error) {
	states, err := s.store.ListStates(userID, targetID)
	if err != nil {
		return nil, fmt.Errorf("notesync list pending deletes: %w", err)
	}
	result := make([]SyncState, 0)
	for _, state := range states {
		if state.PendingDelete {
			result = append(result, state)
		}
	}
	return result, nil
}

func (s *Service) ConfirmDelete(userID, targetID, noteID int64) error {
	state, err := s.store.GetState(userID, noteID, targetID)
	if err != nil {
		return fmt.Errorf("notesync confirm delete load state: %w", err)
	}
	if !state.PendingDelete {
		return errors.New("note is not pending deletion")
	}
	if err := s.notes.SoftDelete(userID, noteID); err != nil {
		return fmt.Errorf("notesync confirm delete note: %w", err)
	}
	now := s.now()
	state.PendingDelete = false
	state.LastDirection = "delete"
	state.LastSyncedAt = &now
	state.UpdatedAt = now
	return s.store.SaveState(state)
}

// RestorePendingDelete restores the external page, unarchives the local note,
// and pushes the local representation back to Notion. It is safe to retry when
// a previous attempt failed after either side was restored.
func (s *Service) RestorePendingDelete(userID, targetID, noteID int64) (*SyncState, error) {
	state, err := s.store.GetState(userID, noteID, targetID)
	if err != nil {
		return nil, fmt.Errorf("notesync restore load state: %w", err)
	}
	if !state.PendingDelete {
		return nil, errors.New("note is not pending deletion")
	}
	target, err := s.store.GetTarget(userID, targetID)
	if err != nil {
		return nil, fmt.Errorf("notesync restore load target: %w", err)
	}
	client, err := s.clientFor(*target)
	if err != nil {
		return nil, err
	}
	if err := client.RestorePage(state.ExternalPageID); err != nil {
		return nil, fmt.Errorf("notesync restore Notion page: %w", err)
	}
	restorer, ok := s.notes.(NoteRestorer)
	if !ok {
		return nil, errors.New("notesync local note store does not support restore")
	}
	if err := restorer.Restore(userID, noteID); err != nil {
		return nil, fmt.Errorf("notesync restore local note: %w", err)
	}
	restored, err := s.PushNote(userID, noteID)
	if err != nil {
		return nil, fmt.Errorf("notesync restore push note: %w", err)
	}
	now := s.now()
	restored.LastDirection = "restore"
	restored.LastSyncedAt = &now
	restored.UpdatedAt = now
	if err := s.store.SaveState(restored); err != nil {
		return nil, fmt.Errorf("notesync restore save state: %w", err)
	}
	return restored, nil
}

func (s *Service) clientFor(target SyncTarget) (NotionClient, error) {
	if s.client == nil {
		return nil, errors.New("notion integration is not configured")
	}
	client, err := s.client(target)
	if err != nil {
		return nil, fmt.Errorf("notesync client: %w", err)
	}
	return client, nil
}

func (s *Service) hasToken(target SyncTarget) bool {
	if strings.TrimSpace(target.TokenEnv) == "" {
		return false
	}
	return strings.TrimSpace(os.Getenv(target.TokenEnv)) != ""
}

func targetFromRequest(userID, id int64, req TargetRequest) (*SyncTarget, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("target name is required")
	}
	cfg, err := notion.NormalizeConfig(notion.Config{DataSourceID: req.DataSourceID, TokenEnv: req.TokenEnv, TitleProperty: req.TitleProperty, TagsProperty: req.TagsProperty, FlowIDProperty: req.FlowIDProperty, SyncTags: req.SyncTags})
	if err != nil {
		return nil, err
	}
	enabled, autoPush := true, false
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.AutoPush != nil {
		autoPush = *req.AutoPush
	}
	return &SyncTarget{ID: id, UserID: userID, Type: TargetTypeNotion, Name: strings.TrimSpace(req.Name), DataSourceID: cfg.DataSourceID, TokenEnv: cfg.TokenEnv, TitleProperty: cfg.TitleProperty, TagsProperty: cfg.TagsProperty, FlowIDProperty: cfg.FlowIDProperty, SyncTags: cfg.SyncTags, Enabled: enabled, AutoPush: autoPush}, nil
}

func pageInput(target SyncTarget, n note.Note, blocks []notion.Block) notion.PageInput {
	return notion.PageInput{DataSourceID: target.DataSourceID, TitleProperty: target.TitleProperty, TagsProperty: target.TagsProperty, FlowIDProperty: target.FlowIDProperty, Title: n.Title, Tags: n.Tags, FlowID: fmt.Sprintf("%d", n.ID), Children: blocks}
}

func parsePageBlocks(blocks []notion.Block) (string, string, error) {
	markdown, err := notion.BlocksToMarkdown(blocks)
	if err != nil {
		return "", "", err
	}
	var contentLines []string
	var source string
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(line, "> 来源：") && source == "" {
			source = strings.TrimSpace(strings.TrimPrefix(line, "> 来源："))
			continue
		}
		contentLines = append(contentLines, line)
	}
	return strings.TrimSpace(strings.Join(contentLines, "\n")), source, nil
}

func pageTitle(page notion.Page, property string) string {
	if property == "" {
		property = "Name"
	}
	return propertyText(page.Properties[property])
}

func pageTags(page notion.Page, property string) []string {
	if property == "" {
		property = "Tags"
	}
	result := make([]string, 0, len(page.Properties[property].MultiSelect))
	for _, option := range page.Properties[property].MultiSelect {
		if value := strings.TrimSpace(option.Name); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func pageTextProperty(page notion.Page, property string) string {
	if property == "" {
		return ""
	}
	return propertyText(page.Properties[property])
}

func propertyText(property notion.PageProperty) string {
	items := property.Title
	if len(items) == 0 {
		items = property.RichText
	}
	var builder strings.Builder
	for _, item := range items {
		if item.PlainText != "" {
			builder.WriteString(item.PlainText)
		} else if item.Text != nil {
			builder.WriteString(item.Text.Content)
		}
	}
	return strings.TrimSpace(builder.String())
}

func matchesSyncTags(tags, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		set[tag] = struct{}{}
	}
	for _, requiredTag := range required {
		if _, ok := set[requiredTag]; ok {
			return true
		}
	}
	return false
}

func parseNoteID(value string) (int64, error) {
	var id int64
	if _, err := fmt.Sscan(strings.TrimSpace(value), &id); err != nil || id <= 0 {
		return 0, errors.New("invalid local note id")
	}
	return id, nil
}

func noteHash(n note.Note) string {
	return hashParts(n.Title, n.Content, n.SourceText, strings.Join(n.Tags, "\x1f"))
}

func externalHash(p ExternalPage) string {
	return hashParts(p.Title, p.Content, p.SourceText, strings.Join(p.Tags, "\x1f"))
}

func hashParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

var _ = time.Second
