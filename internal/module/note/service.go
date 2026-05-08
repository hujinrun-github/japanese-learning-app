package note

import (
	"fmt"
	"log/slog"

	"japanese-learning-app/internal/sm2"
)

// NoteStoreInterface defines data access methods required by NoteService.
type NoteStoreInterface interface {
	Create(note *Note) error
	GetByID(userID, noteID int64) (*Note, error)
	List(userID int64, params NoteListParams) ([]Note, int, error)
	Update(note *Note) error
	SoftDelete(userID, noteID int64) error
	Search(userID int64, query string, limit int) ([]Note, error)
	AddLink(userID, noteID, targetNoteID int64, relation LinkRelation) (*NoteLink, error)
	RemoveLink(userID, linkID int64) error
	GetOutgoingLinks(userID, noteID int64) ([]NoteLink, error)
	GetIncomingLinks(userID, noteID int64) ([]NoteLink, error)
	Promote(userID, noteID int64) error
	Demote(userID, noteID int64) error
	SaveReview(userID, noteID int64, n Note) error
	ListDueNotes(userID int64) ([]Note, error)
	ListArchived(userID int64, params NoteListParams) ([]Note, int, error)
	ListByReference(userID int64, refType string, refID int64, limit int) ([]NoteDigest, error)
	ListTags(userID int64) ([]string, error)
}

// NoteService handles business logic for notes.
type NoteService struct {
	store NoteStoreInterface
}

// NewNoteService creates a NoteService.
func NewNoteService(store NoteStoreInterface) *NoteService {
	return &NoteService{store: store}
}

// Create creates a new note.
func (s *NoteService) Create(n *Note) error {
	slog.Debug("NoteService.Create called", "user_id", n.UserID, "type", n.Type)
	if err := s.store.Create(n); err != nil {
		slog.Error("NoteService.Create failed", "err", err)
		return fmt.Errorf("NoteService.Create: %w", err)
	}
	return nil
}

// GetDetail returns a note with its outgoing and incoming links.
func (s *NoteService) GetDetail(userID, noteID int64) (*NoteDetail, error) {
	slog.Debug("NoteService.GetDetail called", "user_id", userID, "note_id", noteID)

	n, err := s.store.GetByID(userID, noteID)
	if err != nil {
		slog.Error("NoteService.GetDetail: GetByID failed", "err", err)
		return nil, fmt.Errorf("NoteService.GetDetail: %w", err)
	}

	outgoing, err := s.store.GetOutgoingLinks(userID, noteID)
	if err != nil {
		slog.Error("NoteService.GetDetail: GetOutgoingLinks failed", "err", err)
		return nil, fmt.Errorf("NoteService.GetDetail outgoing: %w", err)
	}

	incoming, err := s.store.GetIncomingLinks(userID, noteID)
	if err != nil {
		slog.Error("NoteService.GetDetail: GetIncomingLinks failed", "err", err)
		return nil, fmt.Errorf("NoteService.GetDetail incoming: %w", err)
	}

	return &NoteDetail{
		Note:          *n,
		OutgoingLinks: outgoing,
		IncomingLinks: incoming,
	}, nil
}

// List returns paginated notes with optional filtering.
func (s *NoteService) List(userID int64, params NoteListParams) ([]Note, int, error) {
	slog.Debug("NoteService.List called", "user_id", userID)
	notes, total, err := s.store.List(userID, params)
	if err != nil {
		slog.Error("NoteService.List failed", "err", err)
		return nil, 0, fmt.Errorf("NoteService.List: %w", err)
	}
	return notes, total, nil
}

// Update updates a note.
func (s *NoteService) Update(n *Note) error {
	slog.Debug("NoteService.Update called", "note_id", n.ID)
	if err := s.store.Update(n); err != nil {
		slog.Error("NoteService.Update failed", "err", err)
		return fmt.Errorf("NoteService.Update: %w", err)
	}
	return nil
}

// Delete soft-deletes a note.
func (s *NoteService) Delete(userID, noteID int64) error {
	slog.Debug("NoteService.Delete called", "user_id", userID, "note_id", noteID)
	if err := s.store.SoftDelete(userID, noteID); err != nil {
		slog.Error("NoteService.Delete failed", "err", err)
		return fmt.Errorf("NoteService.Delete: %w", err)
	}
	return nil
}

// Search performs FTS5 search.
func (s *NoteService) Search(userID int64, query string, limit int) ([]Note, error) {
	slog.Debug("NoteService.Search called", "user_id", userID, "query", query)
	notes, err := s.store.Search(userID, query, limit)
	if err != nil {
		slog.Error("NoteService.Search failed", "err", err)
		return nil, fmt.Errorf("NoteService.Search: %w", err)
	}
	return notes, nil
}

// AddLink creates a link between notes.
func (s *NoteService) AddLink(userID, noteID, targetNoteID int64, relation LinkRelation) (*NoteLink, error) {
	slog.Debug("NoteService.AddLink called", "user_id", userID)
	link, err := s.store.AddLink(userID, noteID, targetNoteID, relation)
	if err != nil {
		slog.Error("NoteService.AddLink failed", "err", err)
		return nil, fmt.Errorf("NoteService.AddLink: %w", err)
	}
	return link, nil
}

// RemoveLink deletes a link.
func (s *NoteService) RemoveLink(userID, linkID int64) error {
	slog.Debug("NoteService.RemoveLink called", "user_id", userID)
	if err := s.store.RemoveLink(userID, linkID); err != nil {
		slog.Error("NoteService.RemoveLink failed", "err", err)
		return fmt.Errorf("NoteService.RemoveLink: %w", err)
	}
	return nil
}

// Promote adds the note to the review queue.
func (s *NoteService) Promote(userID, noteID int64) error {
	slog.Debug("NoteService.Promote called", "user_id", userID, "note_id", noteID)
	if err := s.store.Promote(userID, noteID); err != nil {
		slog.Error("NoteService.Promote failed", "err", err)
		return fmt.Errorf("NoteService.Promote: %w", err)
	}
	return nil
}

// Demote removes the note from the review queue.
func (s *NoteService) Demote(userID, noteID int64) error {
	slog.Debug("NoteService.Demote called", "user_id", userID, "note_id", noteID)
	if err := s.store.Demote(userID, noteID); err != nil {
		slog.Error("NoteService.Demote failed", "err", err)
		return fmt.Errorf("NoteService.Demote: %w", err)
	}
	return nil
}

// SubmitRating records a review rating and applies SM-2 scheduling.
// Mastery >= 5 automatically graduates the note (next_review_at = NULL).
func (s *NoteService) SubmitRating(userID, noteID int64, rating sm2.Rating) error {
	slog.Debug("NoteService.SubmitRating called", "user_id", userID, "note_id", noteID, "rating", rating)

	n, err := s.store.GetByID(userID, noteID)
	if err != nil {
		slog.Error("NoteService.SubmitRating: note not found", "err", err)
		return fmt.Errorf("NoteService.SubmitRating: %w", err)
	}

	newMastery, newInterval, newEF, nextReview, newHistory := sm2.CalcNextReview(
		n.MasteryLevel, n.Interval, n.EaseFactor, rating, n.ReviewHistory,
	)

	n.MasteryLevel = newMastery
	n.Interval = newInterval
	n.EaseFactor = newEF
	n.ReviewHistory = newHistory

	if newMastery >= 5 {
		n.NextReviewAt = nil
	} else {
		n.NextReviewAt = &nextReview
	}

	if err := s.store.SaveReview(userID, noteID, *n); err != nil {
		slog.Error("NoteService.SubmitRating: SaveReview failed", "err", err)
		return fmt.Errorf("NoteService.SubmitRating: %w", err)
	}

	return nil
}

// GetReviewQueue returns notes due for review.
func (s *NoteService) GetReviewQueue(userID int64) ([]Note, error) {
	slog.Debug("NoteService.GetReviewQueue called", "user_id", userID)
	notes, err := s.store.ListDueNotes(userID)
	if err != nil {
		slog.Error("NoteService.GetReviewQueue failed", "err", err)
		return nil, fmt.Errorf("NoteService.GetReviewQueue: %w", err)
	}
	return notes, nil
}

// ListArchived returns graduated notes.
func (s *NoteService) ListArchived(userID int64, params NoteListParams) ([]Note, int, error) {
	slog.Debug("NoteService.ListArchived called", "user_id", userID)
	return s.store.ListArchived(userID, params)
}

// ListTags returns all tags used by the user.
func (s *NoteService) ListTags(userID int64) ([]string, error) {
	slog.Debug("NoteService.ListTags called", "user_id", userID)
	tags, err := s.store.ListTags(userID)
	if err != nil {
		slog.Error("NoteService.ListTags failed", "err", err)
		return nil, fmt.Errorf("NoteService.ListTags: %w", err)
	}
	return tags, nil
}

// ListByReference returns note digests referencing a system entity.
func (s *NoteService) ListByReference(userID int64, refType string, refID int64, limit int) ([]NoteDigest, error) {
	slog.Debug("NoteService.ListByReference called", "user_id", userID, "ref_type", refType, "ref_id", refID)
	return s.store.ListByReference(userID, refType, refID, limit)
}

// Recycle brings an archived note back into the review queue (same as Promote).
func (s *NoteService) Recycle(userID, noteID int64) error {
	return s.Promote(userID, noteID)
}
