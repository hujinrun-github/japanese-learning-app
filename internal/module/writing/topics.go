package writing

// WritingTopic offers two ways to practice writing about the same situation.
type WritingTopic struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	JLPTLevel      string `json:"jlpt_level"`
	EssayPrompt    string `json:"essay_prompt"`
	SentencePrompt string `json:"sentence_prompt"`
	Practiced      bool   `json:"practiced"`
}
