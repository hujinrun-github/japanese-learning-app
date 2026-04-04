package data

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/module/lesson"
	"japanese-learning-app/internal/module/summary"
	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
)

// ── WordStoreAdapter ──────────────────────────────────────────────────────────
// Bridges *WordStore (paginated ListByLevel, typed ListDueRecords) to
// word.WordStoreInterface (unpaginated ListByLevel, no-limit ListDueRecords).

// WordStoreAdapter wraps WordStore to satisfy word.WordStoreInterface.
type WordStoreAdapter struct {
	s *WordStore
}

// NewWordStoreAdapter creates a WordStoreAdapter.
func NewWordStoreAdapter(s *WordStore) *WordStoreAdapter {
	return &WordStoreAdapter{s: s}
}

// GetByID delegates to WordStore.GetByID.
func (a *WordStoreAdapter) GetByID(id int64) (*word.Word, error) {
	return a.s.GetByID(id)
}

// ListByLevel fetches all words at the given level (no pagination).
func (a *WordStoreAdapter) ListByLevel(level word.JLPTLevel) ([]word.Word, error) {
	slog.Debug("WordStoreAdapter.ListByLevel called", "level", level)
	words, _, err := a.s.ListByLevel(level, 1, 10000)
	if err != nil {
		return nil, fmt.Errorf("WordStoreAdapter.ListByLevel: %w", err)
	}
	return words, nil
}

// GetRecord returns the word record for the user/word pair.
// Returns (nil, nil) when no record exists yet — the service uses nil to mean "new word".
func (a *WordStoreAdapter) GetRecord(userID, wordID int64) (*word.WordRecord, error) {
	rec, err := a.s.GetRecord(userID, wordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return rec, nil
}

// ListDueRecords fetches up to 200 due records for the user.
func (a *WordStoreAdapter) ListDueRecords(userID int64) ([]word.WordRecord, error) {
	slog.Debug("WordStoreAdapter.ListDueRecords called", "user_id", userID)
	records, err := a.s.ListDueRecords(userID, 200)
	if err != nil {
		return nil, fmt.Errorf("WordStoreAdapter.ListDueRecords: %w", err)
	}
	return records, nil
}

// UpsertRecord delegates to WordStore.UpsertRecord.
func (a *WordStoreAdapter) UpsertRecord(r word.WordRecord) error {
	return a.s.UpsertRecord(r)
}

// BookmarkWord delegates to WordStore.BookmarkWord.
func (a *WordStoreAdapter) BookmarkWord(userID, wordID int64) error {
	return a.s.BookmarkWord(userID, wordID)
}

// ── LessonStoreAdapter ───────────────────────────────────────────────────────
// Bridges *LessonStore (ListSummaries with tag param) to
// lesson.LessonStoreInterface (ListSummaries without tag).

// LessonStoreAdapter wraps LessonStore to satisfy lesson.LessonStoreInterface.
type LessonStoreAdapter struct {
	s *LessonStore
}

// NewLessonStoreAdapter creates a LessonStoreAdapter.
func NewLessonStoreAdapter(s *LessonStore) *LessonStoreAdapter {
	return &LessonStoreAdapter{s: s}
}

// ListSummaries returns all lessons at the given level (no tag filter).
func (a *LessonStoreAdapter) ListSummaries(level lesson.JLPTLevel) ([]lesson.LessonSummary, error) {
	return a.s.ListSummaries(level, "")
}

// GetDetail delegates to LessonStore.GetDetail.
func (a *LessonStoreAdapter) GetDetail(id int64) (*lesson.Lesson, error) {
	return a.s.GetDetail(id)
}

// GetSentences delegates to LessonStore.GetSentences.
func (a *LessonStoreAdapter) GetSentences(lessonID int64) ([]lesson.Sentence, error) {
	return a.s.GetSentences(lessonID)
}

// ── UserStoreAdapter ─────────────────────────────────────────────────────────
// Bridges *UserStore (Create/GetByEmail/GetPasswordHash separately) to
// user.UserStoreInterface (CreateUser/GetUserByEmail returning hash/GetUserByID).

// UserStoreAdapter wraps UserStore to satisfy user.UserStoreInterface.
type UserStoreAdapter struct {
	s *UserStore
}

// NewUserStoreAdapter creates a UserStoreAdapter.
func NewUserStoreAdapter(s *UserStore) *UserStoreAdapter {
	return &UserStoreAdapter{s: s}
}

// CreateUser creates a new user.
func (a *UserStoreAdapter) CreateUser(u user.User, passwordHash string) (*user.User, error) {
	slog.Debug("UserStoreAdapter.CreateUser called", "email", u.Email)
	created, err := a.s.Create(u.Email, passwordHash, u.GoalLevel)
	if err != nil {
		return nil, fmt.Errorf("UserStoreAdapter.CreateUser: %w", err)
	}
	return created, nil
}

// GetUserByEmail returns the user and their stored password hash.
func (a *UserStoreAdapter) GetUserByEmail(email string) (*user.User, string, error) {
	slog.Debug("UserStoreAdapter.GetUserByEmail called", "email", email)
	u, err := a.s.GetByEmail(email)
	if err != nil {
		return nil, "", fmt.Errorf("UserStoreAdapter.GetUserByEmail: %w", err)
	}
	hash, err := a.s.GetPasswordHash(email)
	if err != nil {
		return nil, "", fmt.Errorf("UserStoreAdapter.GetUserByEmail hash: %w", err)
	}
	return u, hash, nil
}

// GetUserByID returns the user by ID.
func (a *UserStoreAdapter) GetUserByID(id int64) (*user.User, error) {
	return a.s.GetByID(id)
}

// ── SessionStoreAdapter ──────────────────────────────────────────────────────
// Bridges *SessionStore (CreateSession returning sessionID, GetSessionData) to
// summary.SummaryStoreInterface (SaveSession, GetSession).

// SessionStoreAdapter wraps SessionStore to satisfy summary.SummaryStoreInterface.
type SessionStoreAdapter struct {
	s *SessionStore
}

// NewSessionStoreAdapter creates a SessionStoreAdapter.
func NewSessionStoreAdapter(s *SessionStore) *SessionStoreAdapter {
	return &SessionStoreAdapter{s: s}
}

// SaveSession persists a study session, ignoring the generated session ID.
func (a *SessionStoreAdapter) SaveSession(sess summary.StudySession) error {
	slog.Debug("SessionStoreAdapter.SaveSession called", "user_id", sess.UserID)
	_, err := a.s.CreateSession(sess)
	if err != nil {
		return fmt.Errorf("SessionStoreAdapter.SaveSession: %w", err)
	}
	return nil
}

// GetSession retrieves a study session by its ID.
func (a *SessionStoreAdapter) GetSession(sessionID string) (*summary.StudySession, error) {
	return a.s.GetSessionData(sessionID)
}

// SaveSummary delegates to SessionStore.SaveSummary.
func (a *SessionStoreAdapter) SaveSummary(sum summary.SessionSummary) error {
	return a.s.SaveSummary(sum)
}

// ListSummaries delegates to SessionStore.ListSummaries.
func (a *SessionStoreAdapter) ListSummaries(userID int64) ([]summary.SessionSummary, error) {
	return a.s.ListSummaries(userID)
}
