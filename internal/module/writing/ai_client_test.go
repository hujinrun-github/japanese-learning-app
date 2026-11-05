package writing_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"japanese-learning-app/internal/module/writing"
)

func TestClaudeReviewUsesTextAfterThinkingBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid test request", http.StatusBadRequest)
			return
		}
		if request.Model != "claude-haiku-5-5" {
			t.Errorf("model = %q", request.Model)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"stop_reason": "end_turn",
			"content": []map[string]string{
				{"type": "thinking", "thinking": "", "signature": "test"},
				{"type": "text", "text": `{"score":88,"reference_answer":"図書館に行きました。"}`},
			},
		}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	client := writing.NewClaudeClient("test-key", server.URL, time.Second)
	feedback, err := client.Review("週末について書いてください。", "図書館に行きました。")
	if err != nil {
		t.Fatal(err)
	}
	if feedback.Score != 88 || feedback.ReferenceAnswer != "図書館に行きました。" {
		t.Fatalf("unexpected feedback: %+v", feedback)
	}
}
