package translation

import "time"

// Direction represents the translation direction.
type Direction string

const (
	DirectionCN2JP Direction = "cn2jp"
	DirectionJP2CN Direction = "jp2cn"
)

// TranslationSource represents an imported source material.
type TranslationSource struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	SourceType  string    `json:"source_type"`  // "manual" | "url" | "api"
	SourceURL   string    `json:"source_url"`
	APIEndpoint string    `json:"api_endpoint"`
	RawContent  string    `json:"raw_content"`
	Direction   string    `json:"direction,omitempty"` // populated after import
	CreatedAt   time.Time `json:"created_at"`
}

// TranslationSentence is a single sentence unit for translation practice.
type TranslationSentence struct {
	ID                   int64  `json:"id"`
	SourceID             int64  `json:"source_id"`
	Direction            string `json:"direction"` // "cn2jp" | "jp2cn"
	SourceText           string `json:"source_text"`
	ReferenceTranslation string `json:"reference_translation"`
	Position             int    `json:"position"`
}

// GrammarExplanation links a grammar point found in the user's translation.
type GrammarExplanation struct {
	GrammarPoint string `json:"grammar_point"`
	Explanation  string `json:"explanation"`
	MatchedDBID  int64  `json:"matched_db_id,omitempty"`
}

// TranslationFeedback is the AI review result.
type TranslationFeedback struct {
	AIScore              int                   `json:"ai_score"`
	GrammarExplanations  []GrammarExplanation  `json:"grammar_explanations"`
	IssueDescription     string                `json:"issue_description"`
	CorrectedTranslation string                `json:"corrected_translation"`
	ReferenceTranslation string                `json:"reference_translation"`
}

// TranslationRecord represents a user's translation practice.
type TranslationRecord struct {
	ID               int64                 `json:"id"`
	UserID           int64                 `json:"user_id"`
	SentenceID       int64                 `json:"sentence_id"`
	UserTranslation  string                `json:"user_translation"`
	Score            int                   `json:"score"`
	RuleScore        int                   `json:"rule_score"`
	AIFeedback       *TranslationFeedback  `json:"ai_feedback,omitempty"`
	PracticedAt      time.Time             `json:"practiced_at"`
}
