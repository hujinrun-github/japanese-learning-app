package translation

import "testing"

func TestAssessTranslation(t *testing.T) {
	tests := []struct {
		name, direction, reference, answer string
		want                               string
	}{
		{"formatting", "jp2cn", "你好。", " 你 好！ ", "reference_match"},
		{"Japanese bus fare paraphrase", "cn2jp", "バス賃はいくらですか？", "バスの運賃はいくらでしょうか。", "equivalent"},
		{"Chinese bus fare paraphrase", "jp2cn", "公车票价多少？", "公交车的票价是多少钱？", "equivalent"},
		{"same regret expressed differently", "jp2cn", "如果没下雨，我就去钓鱼了。", "要是没有下雨，我就去钓鱼了。", "equivalent"},
		{"negation changes meaning", "jp2cn", "我今天不去。", "我今天要去。", "meaning_changed"},
		{"Japanese polarity changes meaning", "cn2jp", "今日は行きません。", "今日は行きます。", "meaning_changed"},
		{"unclear polarity remains unrecognized", "jp2cn", "我不觉得他会来。", "我觉得他不会来。", "unrecognized"},
		{"missing qualifier", "jp2cn", "我今天只买了一本书。", "我买了一本书。", "unrecognized"},
		{"do not merge different financial amounts", "jp2cn", "温度是-5度。", "温度是5度。", "unrecognized"},
		{"do not merge different decimals", "jp2cn", "价格是1.5元。", "价格是15元。", "unrecognized"},
		{"do not merge separated digits", "jp2cn", "编号是1 2。", "编号是12。", "unrecognized"},
		{"question and statement", "jp2cn", "你今天来？", "你今天来。", "unrecognized"},
		{"addition changes meaning", "cn2jp", "バス賃はいくらですか？", "今日のバス代はいくらですか？", "unrecognized"},
		{"missing reference", "cn2jp", "", "こんにちは。", "no_reference"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := assessTranslation(tc.direction, tc.reference, tc.answer); string(got) != tc.want {
				t.Errorf("assessment = %q, want %q", got, tc.want)
			}
		})
	}
}
