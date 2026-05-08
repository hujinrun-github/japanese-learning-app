package note

import (
	"time"

	"japanese-learning-app/internal/sm2"
)

// NoteType classifies the kind of content a note captures.
type NoteType string

const (
	TypeWord     NoteType = "word"
	TypeGrammar  NoteType = "grammar"
	TypeSentence NoteType = "sentence"
)

// LinkRelation describes how two notes are related.
type LinkRelation string

const (
	RelationRelated     LinkRelation = "related"
	RelationUsesWord    LinkRelation = "uses_word"
	RelationUsesGrammar LinkRelation = "uses_grammar"
	RelationContext     LinkRelation = "context"
)

// Note is a user-private record of a word, grammar point, or sentence.
type Note struct {
	ID            int64            `json:"id"`
	UserID        int64            `json:"-"`
	Type          NoteType         `json:"type"`
	Title         string           `json:"title"`
	Content       string           `json:"content"`
	SourceText    string           `json:"source_text"`
	ReferenceID   *int64           `json:"reference_id,omitempty"`
	ReferenceType *string          `json:"reference_type,omitempty"`
	Tags          []string         `json:"tags"`
	MasteryLevel  int              `json:"mastery_level"`
	NextReviewAt  *time.Time       `json:"next_review_at,omitempty"`
	EaseFactor    float64          `json:"ease_factor"`
	Interval      int              `json:"interval"`
	ReviewHistory []sm2.ReviewEvent `json:"review_history"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// NoteLink connects two notes with a typed relationship.
type NoteLink struct {
	ID           int64        `json:"id"`
	NoteID       int64        `json:"note_id"`
	TargetNoteID int64        `json:"target_note_id"`
	Relation     LinkRelation `json:"relation"`
	TargetNote   *NoteDigest  `json:"target_note,omitempty"`
}

// NoteDetail is the full view of a note including all inbound and outbound links.
type NoteDetail struct {
	Note
	OutgoingLinks []NoteLink `json:"links"`
	IncomingLinks []NoteLink `json:"backlinks"`
}

// NoteDigest is a lightweight reference to another note, used in link enrichment and cross-module lists.
type NoteDigest struct {
	ID    int64    `json:"id"`
	Title string   `json:"title"`
	Type  NoteType `json:"type"`
}

// NoteListParams filters and sorts note list queries.
type NoteListParams struct {
	Type   NoteType
	Tag    string
	Sort   string // "created_at" | "updated_at" | "next_review_at"
	Order  string // "asc" | "desc"
	Offset int
	Limit  int
}
