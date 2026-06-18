package shadowing

import "japanese-learning-app/internal/module/lesson"

// CurrentSentenceIndex returns the lesson sentence index active at currentTimeMS.
func CurrentSentenceIndex(sentences []lesson.Sentence, currentTimeMS int64) (int, bool) {
	valid := make([]lesson.Sentence, 0, len(sentences))
	for _, sentence := range sentences {
		if sentence.EndMS > sentence.StartMS {
			valid = append(valid, sentence)
		}
	}
	if len(valid) == 0 {
		return -1, false
	}

	if currentTimeMS < valid[0].StartMS {
		return valid[0].Index, true
	}

	current := valid[0]
	for _, sentence := range valid {
		if currentTimeMS < sentence.StartMS {
			return current.Index, true
		}
		current = sentence
		if currentTimeMS < sentence.EndMS {
			return sentence.Index, true
		}
	}

	return current.Index, true
}
