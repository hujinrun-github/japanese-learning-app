package notion

import (
	"strings"
	"testing"
)

func TestMarkdownBlocksRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "basic blocks", content: "# 标题\n正文\n- 项目\n- [x] 完成\n> 引用\n---"},
		{name: "source text", content: "学习内容"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, err := MarkdownToBlocks(tt.content, "例句")
			if err != nil {
				t.Fatalf("MarkdownToBlocks failed: %v", err)
			}
			got, err := BlocksToMarkdown(blocks)
			if err != nil {
				t.Fatalf("BlocksToMarkdown failed: %v", err)
			}
			if !strings.HasPrefix(got, "> 来源：例句") {
				t.Fatalf("round-trip output = %q", got)
			}
		})
	}
}

func TestMarkdownToBlocksRejectsUnsupportedCodeFence(t *testing.T) {
	if _, err := MarkdownToBlocks("```go\nfmt.Println(1)\n```", ""); err == nil {
		t.Fatal("expected standalone code fence to be rejected")
	}
}
