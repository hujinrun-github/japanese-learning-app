package translation

import (
	"time"

	"japanese-learning-app/internal/module/word"
)

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
	SourceType  string    `json:"source_type"` // "manual" | "url" | "api"
	SourceURL   string    `json:"source_url"`
	APIEndpoint string    `json:"api_endpoint"`
	RawContent  string    `json:"raw_content"`
	JLPTLevels  []string  `json:"jlpt_levels,omitempty"`
	Direction   string    `json:"direction,omitempty"` // populated after import
	CreatedAt   time.Time `json:"created_at"`
}

// TranslationSentence is a single sentence unit for translation practice.
type TranslationSentence struct {
	ID                   int64    `json:"id"`
	SourceID             int64    `json:"source_id"`
	Direction            string   `json:"direction"` // "cn2jp" | "jp2cn"
	SourceText           string   `json:"source_text"`
	ReferenceTranslation string   `json:"reference_translation"`
	Position             int      `json:"position"`
	JLPTLevel            string   `json:"jlpt_level,omitempty"`
	GrammarPoints        []string `json:"-"`
}

// GrammarExplanation links a grammar point found in the user's translation.
type GrammarExplanation struct {
	GrammarPoint string `json:"grammar_point"`
	Explanation  string `json:"explanation"`
	Evidence     string `json:"evidence,omitempty"`
	MatchedDBID  int64  `json:"matched_db_id,omitempty"`
}

// TranslationFeedback preserves feedback from previously graded records.
type TranslationFeedback struct {
	AIScore              int                  `json:"ai_score"`
	GrammarExplanations  []GrammarExplanation `json:"grammar_explanations"`
	GrammarSource        string               `json:"grammar_source,omitempty"`
	IssueDescription     string               `json:"issue_description"`
	CorrectedTranslation string               `json:"corrected_translation"`
	ReferenceTranslation string               `json:"reference_translation"`
}

// TranslationAssessment is the local result saved with a newly submitted answer.
type TranslationAssessment struct {
	Result              AssessmentResult     `json:"result"`
	GrammarExplanations []GrammarExplanation `json:"grammar_explanations,omitempty"`
	GrammarSource       string               `json:"grammar_source,omitempty"`
}

// TranslationRecord represents a user's translation practice.
type TranslationRecord struct {
	ID                   int64                  `json:"id"`
	UserID               int64                  `json:"user_id"`
	SentenceID           int64                  `json:"sentence_id"`
	Direction            string                 `json:"-"`
	UserTranslation      string                 `json:"user_translation"`
	ReferenceTranslation string                 `json:"reference_translation,omitempty"`
	ReferenceTokens      []word.FuriganaToken   `json:"reference_tokens,omitempty"`
	Score                int                    `json:"score,omitempty"`
	RuleScore            int                    `json:"rule_score,omitempty"`
	AIFeedback           *TranslationFeedback   `json:"ai_feedback,omitempty"`
	Assessment           *TranslationAssessment `json:"assessment,omitempty"`
	KnowledgeLinks       []SentenceLink         `json:"knowledge_links,omitempty"`
	PracticedAt          time.Time              `json:"practiced_at"`
}

// DailyProgress is the learner's effective translation target and progress.
type DailyProgress struct {
	Goal      int      `json:"daily_goal"`
	Completed int      `json:"today_completed"`
	Levels    []string `json:"jlpt_levels"`
}
