package word

import "strings"

// 动词自他标签取值。线上词库把自他信息混写在 part_of_speech 里（如「自动词」、
// 「自他動2」、「ナ形・名・自動3」），这里统一成独立字段的规范值。
const (
	VerbTypeIntransitive = "自动词"
	VerbTypeTransitive   = "他动词"
	VerbTypeBoth         = "自他两用"
)

// NormalizeVerbType 从词性字符串中提取自他动词标签。
// 词性里没有自他信息（如「動詞」「一段动词」「形容動詞」）时返回空字符串。
func NormalizeVerbType(partOfSpeech string) string {
	pos := strings.TrimSpace(partOfSpeech)
	switch {
	case strings.Contains(pos, "自他動"), strings.Contains(pos, "自他动"), strings.Contains(pos, "自他两用"):
		return VerbTypeBoth
	case strings.Contains(pos, "自動"), strings.Contains(pos, "自动词"):
		return VerbTypeIntransitive
	case strings.Contains(pos, "他動"), strings.Contains(pos, "他动词"):
		return VerbTypeTransitive
	default:
		return ""
	}
}

// IsVerbType 报告 label 是否为合法的 verb_type 取值（空值表示非动词或尚未标注）。
func IsVerbType(label string) bool {
	switch label {
	case "", VerbTypeIntransitive, VerbTypeTransitive, VerbTypeBoth:
		return true
	default:
		return false
	}
}
