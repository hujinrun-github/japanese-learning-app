package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompleteChatAgainstMockServer(t *testing.T) {
	var gotAuth string
	var gotBody string
	var calls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		gotAuth = r.Header.Get("Authorization")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"conjunction_rule\":\"动词た形 + あげく\"}"}}]}`))
	}))
	defer srv.Close()

	text, err := completeChat(context.Background(), GrammarLLMConfig{
		Endpoint: srv.URL,
		APIKey:   "test-key",
		Model:    "mock-model",
		Timeout:  5 * time.Second,
	}, srv.Client(), "hello")
	if err != nil {
		t.Fatalf("completeChat: %v", err)
	}
	if !strings.Contains(text, "动词た形") {
		t.Fatalf("unexpected content: %q", text)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if !strings.Contains(gotBody, "mock-model") {
		t.Fatalf("request body missing model: %s", gotBody)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("server calls = %d, want 1", calls)
	}
}

func TestCompleteChatSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"message":"Insufficient Balance"}}`))
	}))
	defer srv.Close()

	_, err := completeChat(context.Background(), GrammarLLMConfig{
		Endpoint: srv.URL,
		Timeout:  5 * time.Second,
	}, srv.Client(), "hello")
	if err == nil {
		t.Fatal("completeChat expected error on 402")
	}
	if !strings.Contains(err.Error(), "Insufficient Balance") {
		t.Fatalf("error = %v, want to mention Insufficient Balance", err)
	}
}

func TestParseLLMGenerated(t *testing.T) {
	raw := "```json\n" + `{
      "conjunction_rule": "动词普通形 + にもかかわらず",
      "usage_note": "表示与预期相反的结果，书面语。",
      "examples": [{"japanese": "雨にもかかわらず、試合を行った。", "chinese": "尽管下雨，还是进行了比赛。"}]
    }` + "\n```"

	got, err := parseLLMGenerated(raw)
	if err != nil {
		t.Fatalf("parseLLMGenerated: %v", err)
	}
	if got.ConjunctionRule != "动词普通形 + にもかかわらず" {
		t.Fatalf("conjunction_rule = %q", got.ConjunctionRule)
	}
	if got.UsageNote != "表示与预期相反的结果，书面语。" {
		t.Fatalf("usage_note = %q", got.UsageNote)
	}
	if len(got.Examples) != 1 || got.Examples[0].Japanese == "" {
		t.Fatalf("examples = %+v", got.Examples)
	}
}

func TestParseLLMGeneratedRejectsGarbage(t *testing.T) {
	if _, err := parseLLMGenerated("抱歉，我无法回答"); err == nil {
		t.Fatal("parseLLMGenerated expected error for non-JSON input")
	}
}

func TestBuildGrammarPrompt(t *testing.T) {
	point := grammarPointRow{
		Name:    "〜あげく",
		Meaning: "结果…；最后…",
		Level:   "N2",
		Examples: []grammarExampleRow{
			{Position: 0, Japanese: "相談したあげく、辞めた。", Chinese: "商量的结果，辞职了。"},
		},
	}

	prompt := buildGrammarPrompt(point, 3)
	for _, want := range []string{"〜あげく", "N2", "结果…；最后…", "相談したあげく、辞めた。", "conjunction_rule", "usage_note", "examples"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildGrammarPromptWithoutExamples(t *testing.T) {
	prompt := buildGrammarPrompt(grammarPointRow{Name: "〜ばかり", Level: "N3", Meaning: "只…；光…"}, 2)
	if strings.Contains(prompt, "已有例句：") {
		t.Fatalf("prompt should not list existing examples:\n%s", prompt)
	}
	for _, want := range []string{"〜ばかり", "N3", "只…；光…", "examples"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdef", 10); got != "abcdef" {
		t.Fatalf("truncate short = %q", got)
	}
	long := strings.Repeat("あ", 10)
	got := truncate(long, 3)
	if len([]rune(got)) != 4 { // 3 runes + ellipsis
		t.Fatalf("truncate long = %q, want 4 runes", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncate long = %q, want ellipsis suffix", got)
	}
}
