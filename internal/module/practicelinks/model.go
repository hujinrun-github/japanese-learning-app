package practicelinks

import (
	"context"
	"log/slog"
)

const (
	ListeningMaterial = "listening-material"
	ListeningSentence = "listening-sentence"
	ListeningQuestion = "listening-question"
	SpeakingMaterial  = "speaking-material"
	WritingTopic      = "writing-topic"
	WritingQuestion   = "writing-question"
	Word              = "word"
	Grammar           = "grammar"
	MaxLinks          = 30
)

type Ref struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

type Link struct {
	Ref
	Title     string `json:"title"`
	Reading   string `json:"reading,omitempty"`
	Meaning   string `json:"meaning"`
	JLPTLevel string `json:"jlpt_level"`
	Hint      bool   `json:"hint,omitempty"`
}

type Selection struct {
	Ref
	Hint bool `json:"hint,omitempty"`
}

type Target struct {
	Kind string
	ID   string
}

type Source struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Context string `json:"context"`
	Level   string `json:"jlpt_level"`
	CanHint bool   `json:"can_hint"`
}

type ListeningLinks struct {
	Material  []Link            `json:"material,omitempty"`
	Sentences map[int][]Link    `json:"sentences,omitempty"`
	Questions map[string][]Link `json:"questions,omitempty"`
}

type Reader interface {
	ListLinks(ctx context.Context, target Target, hintsOnly bool) ([]Link, error)
}

type AdminStore interface {
	Reader
	ListSources(ctx context.Context, kind, query string, offset, limit int) ([]Source, error)
	SourceExists(ctx context.Context, target Target) error
	ReplaceLinks(ctx context.Context, target Target, links []Selection) error
}

func AfterSubmission(ctx context.Context, reader Reader, target Target) []Link {
	if reader == nil || target.ID == "" {
		return nil
	}
	links, err := reader.ListLinks(ctx, target, false)
	if err != nil {
		slog.ErrorContext(ctx, "load knowledge links after submission", "source_kind", target.Kind, "source_id", target.ID, "err", err)
		return nil
	}
	return links
}
