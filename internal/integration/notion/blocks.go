package notion

import (
	"errors"
	"fmt"
	"strings"
)

// MarkdownToBlocks converts the intentionally small Markdown subset used by the
// local note editor into Notion blocks.
func MarkdownToBlocks(content, sourceText string) ([]Block, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	blocks := make([]Block, 0, len(lines)+1)
	if strings.TrimSpace(sourceText) != "" {
		blocks = append(blocks, textBlock("quote", "来源："+strings.TrimSpace(sourceText)))
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		switch {
		case trimmed == "---":
			blocks = append(blocks, Block{"object": "block", "type": "divider", "divider": map[string]any{}})
		case strings.HasPrefix(trimmed, "### "):
			blocks = append(blocks, textBlock("heading_3", strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))))
		case strings.HasPrefix(trimmed, "## "):
			blocks = append(blocks, textBlock("heading_2", strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))))
		case strings.HasPrefix(trimmed, "# "):
			blocks = append(blocks, textBlock("heading_1", strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))))
		case strings.HasPrefix(trimmed, "- [ ] "):
			blocks = append(blocks, checkboxBlock(false, strings.TrimSpace(strings.TrimPrefix(trimmed, "- [ ] "))))
		case strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] "):
			prefix := "- [x] "
			if strings.HasPrefix(trimmed, "- [X] ") {
				prefix = "- [X] "
			}
			blocks = append(blocks, checkboxBlock(true, strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))))
		case strings.HasPrefix(trimmed, "- "):
			blocks = append(blocks, textBlock("bulleted_list_item", strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))))
		case strings.HasPrefix(trimmed, "> "):
			blocks = append(blocks, textBlock("quote", strings.TrimSpace(strings.TrimPrefix(trimmed, "> "))))
		case strings.HasPrefix(trimmed, "```"):
			return nil, errors.New("standalone code fences are not supported; use a code block without fences")
		default:
			blocks = append(blocks, textBlock("paragraph", trimmed))
		}
	}
	return blocks, nil
}

func BlocksToMarkdown(blocks []Block) (string, error) {
	var lines []string
	for _, block := range blocks {
		blockType, _ := block["type"].(string)
		payload, _ := block[blockType].(map[string]any)
		text := blockText(payload)
		switch blockType {
		case "paragraph":
			lines = append(lines, text)
		case "heading_1":
			lines = append(lines, "# "+text)
		case "heading_2":
			lines = append(lines, "## "+text)
		case "heading_3":
			lines = append(lines, "### "+text)
		case "bulleted_list_item":
			lines = append(lines, "- "+text)
		case "numbered_list_item":
			lines = append(lines, "1. "+text)
		case "to_do":
			checked, _ := payload["checked"].(bool)
			mark := "[ ]"
			if checked {
				mark = "[x]"
			}
			lines = append(lines, "- "+mark+" "+text)
		case "quote":
			if strings.HasPrefix(text, "来源：") {
				lines = append(lines, "> "+text)
			} else {
				lines = append(lines, "> "+text)
			}
		case "divider":
			lines = append(lines, "---")
		case "code":
			lines = append(lines, text)
		default:
			return "", fmt.Errorf("unsupported notion block type %q", blockType)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func textBlock(blockType, text string) Block {
	return Block{
		"object": "block",
		"type":   blockType,
		blockType: map[string]any{
			"rich_text": []map[string]any{{"type": "text", "text": map[string]any{"content": text}}},
		},
	}
}

func checkboxBlock(checked bool, text string) Block {
	return Block{
		"object": "block",
		"type":   "to_do",
		"to_do": map[string]any{
			"checked":   checked,
			"rich_text": []map[string]any{{"type": "text", "text": map[string]any{"content": text}}},
		},
	}
}

func blockText(payload map[string]any) string {
	var items []any
	switch raw := payload["rich_text"].(type) {
	case []any:
		items = raw
	case []map[string]any:
		items = make([]any, len(raw))
		for i := range raw {
			items[i] = raw[i]
		}
	default:
		return ""
	}
	var builder strings.Builder
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if plain, ok := item["plain_text"].(string); ok {
			builder.WriteString(plain)
			continue
		}
		if textPayload, ok := item["text"].(map[string]any); ok {
			if content, ok := textPayload["content"].(string); ok {
				builder.WriteString(content)
			}
		}
	}
	return builder.String()
}
