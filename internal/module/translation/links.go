package translation

import "japanese-learning-app/internal/module/practicelinks"

const (
	SentenceLinkWord    = practicelinks.Word
	SentenceLinkGrammar = practicelinks.Grammar
	MaxSentenceLinks    = practicelinks.MaxLinks
)

type SentenceLinkRef = practicelinks.Ref
type SentenceLink = practicelinks.Link
