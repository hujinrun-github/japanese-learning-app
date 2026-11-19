package speaking

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
	"golang.org/x/text/unicode/norm"
)

// WordPronunciation addresses the original material using Unicode codepoints;
// End is exclusive. Judgements are based on ASR readings, not phoneme analysis.
type WordPronunciation struct {
	Start      int    `json:"start"`
	End        int    `json:"end"`
	Surface    string `json:"surface"`
	Reading    string `json:"reading"`
	Recognized string `json:"recognized,omitempty"`
	Status     string `json:"status"`
}

type PronunciationFeedback struct {
	Status  string              `json:"status"`
	Reason  string              `json:"reason,omitempty"`
	Message string              `json:"message,omitempty"`
	Words   []WordPronunciation `json:"words"`
}

// PronunciationAnalyzer owns its tokenizer and ASR dependency. Kagome creates a
// new lattice per Tokenize call, so concurrent requests do not share state.
type PronunciationAnalyzer struct {
	scorer    ASRScorer
	tokenizer *tokenizer.Tokenizer
}

func NewPronunciationAnalyzer(scorer ASRScorer) (*PronunciationAnalyzer, error) {
	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		slog.Error("speaking: initialize pronunciation tokenizer failed", "err", err)
		return nil, fmt.Errorf("speaking.NewPronunciationAnalyzer tokenizer: %w", err)
	}
	return &PronunciationAnalyzer{scorer: scorer, tokenizer: t}, nil
}

func (a *PronunciationAnalyzer) Analyze(ctx context.Context, audio []byte, reference string) (PronunciationFeedback, string) {
	if a == nil || a.scorer == nil {
		return PronunciationFeedback{Status: "unavailable", Message: "尚未配置语音识别服务，暂不能判断单词读音。", Words: []WordPronunciation{}}, ""
	}
	_, recognized, err := a.scorer.ScoreAudio(ctx, audio, reference)
	if err != nil {
		slog.Error("speaking: pronunciation recognition failed", "err", err, "audio_bytes", len(audio))
		return PronunciationFeedback{Status: "error", Message: "语音识别暂时失败，请稍后重新录音；本次仍保留节奏、语调和流利度评分。", Words: []WordPronunciation{}}, ""
	}
	return a.Compare(reference, recognized), recognized
}

// Compare aligns pronunciation characters rather than token positions. This
// allows ASR to use kana or different token boundaries, and a missing word does
// not shift every subsequent judgement. Punctuation and printed speaker labels
// are omitted exactly as they are when the textbook audio is generated.
func (a *PronunciationAnalyzer) Compare(reference, recognized string) PronunciationFeedback {
	words, ref, owners := a.readingTokens(reference, true)
	_, rec, _ := a.readingTokens(recognized, false)
	// Keep traceback memory and CPU bounded for malformed provider responses.
	if len(ref) > 8192 || len(rec) > 8192 || (len(ref)+1)*(len(rec)+1) > 32<<20 {
		slog.Error("speaking: pronunciation text exceeds alignment limit", "reference_runes", len(ref), "recognized_runes", len(rec))
		return PronunciationFeedback{Status: "error", Message: "识别文本过长，无法生成逐词标记，请分段练习。", Words: []WordPronunciation{}}
	}
	if len(ref) == 0 {
		return PronunciationFeedback{Status: "error", Message: "素材没有可识别的朗读内容。", Words: []WordPronunciation{}}
	}
	width := len(rec) + 1
	trace := make([]byte, (len(ref)+1)*width)
	previous, current := make([]int, width), make([]int, width)
	for j := 1; j < width; j++ {
		previous[j] = j
		trace[j] = 'i'
	}
	for i := 1; i <= len(ref); i++ {
		current[0] = i
		trace[i*width] = 'd'
		for j := 1; j < width; j++ {
			cost := 0
			if ref[i-1] != rec[j-1] {
				cost = 1
			}
			best, direction := previous[j-1]+cost, byte('s')
			if previous[j]+1 < best {
				best, direction = previous[j]+1, 'd'
			}
			if current[j-1]+1 < best {
				best, direction = current[j-1]+1, 'i'
			}
			current[j] = best
			trace[i*width+j] = direction
		}
		previous, current = current, previous
	}
	// Build aligned recognized readings backwards, then reverse each word's
	// runes. Insertions belong to the preceding word and do not shift offsets.
	aligned := make([][]rune, len(words))
	errors := make([]bool, len(words))
	for i, j := len(ref), len(rec); i > 0 || j > 0; {
		switch trace[i*width+j] {
		case 's':
			owner := owners[i-1]
			aligned[owner] = append(aligned[owner], rec[j-1])
			if ref[i-1] != rec[j-1] {
				errors[owner] = true
			}
			i--
			j--
		case 'd':
			errors[owners[i-1]] = true
			i--
		case 'i':
			owner := 0
			if i > 0 {
				owner = owners[i-1]
			}
			aligned[owner] = append(aligned[owner], rec[j-1])
			errors[owner] = true
			j--
		}
	}
	for i := range words {
		for left, right := 0, len(aligned[i])-1; left < right; left, right = left+1, right-1 {
			aligned[i][left], aligned[i][right] = aligned[i][right], aligned[i][left]
		}
		words[i].Recognized = string(aligned[i])
		words[i].Status = "correct"
		if len(aligned[i]) == 0 {
			words[i].Status = "missing"
		} else if errors[i] {
			words[i].Status = "mismatch"
		}
	}
	slog.Debug("speaking: pronunciation alignment complete", "words", len(words), "reference_runes", len(ref), "recognized_runes", len(rec))
	return PronunciationFeedback{Status: "available", Words: words}
}

func (a *PronunciationAnalyzer) readingTokens(text string, stripLabels bool) ([]WordPronunciation, []rune, []int) {
	words := []WordPronunciation{}
	var reading []rune
	var owners []int
	lineOffset := 0
	for _, raw := range strings.SplitAfter(text, "\n") {
		line := strings.TrimRight(raw, "\r\n")
		prefix := 0
		if stripLabels {
			prefix = spokenLineStart([]rune(line))
		}
		lineRunes := []rune(line)
		tokenText := []rune(string(lineRunes[prefix:]))
		for i, r := range tokenText {
			if r >= '０' && r <= '９' {
				tokenText[i] = r - '０' + '0'
			}
		}
		for _, tok := range a.tokenizer.Tokenize(string(tokenText)) {
			pronunciation, ok := tok.Pronunciation()
			if !ok || pronunciation == "*" {
				pronunciation = tok.Surface
			}
			standardReading, ok := tok.Reading()
			if !ok || standardReading == "*" {
				standardReading = tok.Surface
			}
			if number, ok := smallIntegerKanji(tok.Surface); ok {
				var pronounced, display strings.Builder
				for _, n := range a.tokenizer.Tokenize(number) {
					p, hasPronunciation := n.Pronunciation()
					if !hasPronunciation || p == "*" {
						p = n.Surface
					}
					r, hasReading := n.Reading()
					if !hasReading || r == "*" {
						r = n.Surface
					}
					pronounced.WriteString(p)
					display.WriteString(r)
				}
				pronunciation, standardReading = pronounced.String(), display.String()
			}
			phonetic := normalizeLongVowels(normalizePronunciation(pronunciation))
			if phonetic == "" {
				continue
			}
			word := WordPronunciation{
				Start: lineOffset + prefix + tok.Start, End: lineOffset + prefix + tok.End,
				Surface: string(lineRunes[prefix+tok.Start : prefix+tok.End]), Reading: normalizePronunciation(standardReading),
			}
			index := len(words)
			words = append(words, word)
			for _, r := range phonetic {
				reading = append(reading, r)
				owners = append(owners, index)
			}
		}
		lineOffset += utf8.RuneCountInString(raw)
	}
	return words, reading, owners
}

// Normalize ordinary integer spellings (3 / 三, 2026 / 二千二十六) through
// the same dictionary. Dates, counters, decimals and digit-by-digit identifiers
// need context-dependent readings and are deliberately not inferred here.
func smallIntegerKanji(text string) (string, bool) {
	if len(text) == 0 || len(text) > 4 || (len(text) > 1 && text[0] == '0') {
		return "", false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	if text == "0" {
		return "零", true
	}
	digits := []rune("零一二三四五六七八九")
	units := []rune("十百千")
	var result strings.Builder
	for i, r := range text {
		if r == '0' {
			continue
		}
		position := len(text) - i - 1
		if r != '1' || position == 0 {
			result.WriteRune(digits[r-'0'])
		}
		if position > 0 {
			result.WriteRune(units[position-1])
		}
	}
	return result.String(), true
}

func spokenLineStart(line []rune) int {
	i := 0
	for i < len(line) && unicode.IsSpace(line[i]) {
		i++
	}
	if i+1 < len(line) && line[i] >= '1' && line[i] <= '4' && (line[i+1] == '.' || line[i+1] == '．') {
		i += 2
		for i < len(line) && unicode.IsSpace(line[i]) {
			i++
		}
	}
	for j := i; j < len(line) && j-i <= 8; j++ {
		if line[j] == '：' || line[j] == ':' {
			if j > i {
				i = j + 1
			}
			break
		}
	}
	return i
}

func normalizePronunciation(text string) string {
	var out strings.Builder
	for _, r := range norm.NFKC.String(text) {
		if r >= 'ァ' && r <= 'ヶ' {
			r -= 0x60
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == 'ー' {
			out.WriteRune(unicode.ToLower(r))
		}
	}
	return out.String()
}

// IPA may emit トーキョー for 東京 but トウキョー for kana とうきょう.
// Expand both long-vowel spellings to the same sound sequence. This is an ASR
// reading comparison; lexical homophones and pitch accent stay indistinguishable.
func normalizeLongVowels(text string) string {
	out := []rune(text)
	for i := 1; i < len(out); i++ {
		vowel := kanaVowel(out[i-1])
		if out[i] == 'ー' && vowel != 0 {
			out[i] = vowel
		} else if vowel == 'お' && out[i] == 'う' {
			out[i] = 'お'
		} else if vowel == 'え' && out[i] == 'い' {
			out[i] = 'え'
		}
	}
	return string(out)
}

func kanaVowel(r rune) rune {
	for _, group := range []struct {
		kana  string
		vowel rune
	}{
		{"ぁあかがさざただなはばぱまゃやらゎわ", 'あ'},
		{"ぃいきぎしじちぢにひびぴみりゐ", 'い'},
		{"ぅうくぐすずつづぬふぶぷむゅゆるゔ", 'う'},
		{"ぇえけげせぜてでねへべぺめれゑ", 'え'},
		{"ぉおこごそぞとどのほぼぽもょよろを", 'お'},
	} {
		if strings.ContainsRune(group.kana, r) {
			return group.vowel
		}
	}
	return 0
}
