package errata

import "strings"

// 勘误的「题目类型」由两个维度共同决定：module 决定定位键 item_key 的形态，
// category（问题类别）决定用户能改哪些字段、以及能否自行撤回报告。

// validItemKeyForModule 按练习模块校验定位键。各模块的题目定位方式不同：
// word/writing 用练习模式，grammar 用测验题，lesson/shadowing 用句子，
// speaking/translation 只有材料或句子编号。
func validItemKeyForModule(module, key string) bool {
	if !validItemKey(key) {
		return false
	}
	parts := strings.Split(key, ":")
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	switch module {
	case "word":
		return len(parts) == 1 || (len(parts) == 2 && (parts[1] == "recognition" || parts[1] == "dictation"))
	case "grammar":
		return len(parts) == 1 || (len(parts) == 3 && parts[1] == "quiz")
	case "lesson":
		return len(parts) == 1 || (len(parts) == 3 && parts[1] == "sentence")
	case "speaking", "translation":
		return len(parts) == 1
	case "shadowing":
		if len(parts) == 1 {
			return true
		}
		if len(parts) != 4 || parts[2] != "sentence" {
			return false
		}
		version := parts[1]
		if len(version) < 2 || version[0] != 'v' {
			return false
		}
		for _, digit := range version[1:] {
			if digit < '0' || digit > '9' {
				return false
			}
		}
		return true
	case "writing":
		return len(parts) == 2 && (parts[1] == "essay" || parts[1] == "sentence")
	default:
		return false
	}
}

// editableFields 返回该问题类别是否允许修改题面摘要与定位键。
// 文本类（题目、答案、解析）由用户手工描述，允许一并修正定位信息；
// 音频/视频与其他类的定位信息由客户端采集，用户只能补充说明文字。
func editableFields(category string) (label, key bool) {
	switch category {
	case "prompt", "answer", "explanation":
		return true, true
	case "audio", "other":
		return false, false
	default:
		return false, false
	}
}

// learnerDeletable 判断用户能否自行撤回该类勘误。
// 音频/视频类会触发媒体资源核查，交由管理员在处理列表中跟进，
// 不允许提交者中途删除；其余类别（含误报的 other）可自行撤回。
func learnerDeletable(category string) bool {
	return category != "audio"
}
