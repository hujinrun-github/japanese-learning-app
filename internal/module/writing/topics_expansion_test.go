package writing_test

import (
	"regexp"
	"strings"
	"testing"

	pgdata "japanese-learning-app/internal/data/postgres"
	"japanese-learning-app/internal/testdb"
)

type seededWritingTopic struct {
	ID             string
	Title          string
	Level          string
	EssayPrompt    string
	SentencePrompt string
	Position       int
}

func loadSeededWritingTopics(t *testing.T) []seededWritingTopic {
	t.Helper()
	db := testdb.Open(t, (pgdata.Adapter{}).RunMigrations)
	rows, err := db.Query(`SELECT id,title,jlpt_level,essay_prompt,sentence_prompt,position
		FROM writing_topics ORDER BY position,id`)
	if err != nil {
		t.Fatalf("query writing_topics: %v", err)
	}
	defer rows.Close()

	var topics []seededWritingTopic
	for rows.Next() {
		var topic seededWritingTopic
		if err := rows.Scan(&topic.ID, &topic.Title, &topic.Level, &topic.EssayPrompt,
			&topic.SentencePrompt, &topic.Position); err != nil {
			t.Fatalf("scan writing_topics: %v", err)
		}
		topics = append(topics, topic)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate writing_topics: %v", err)
	}
	return topics
}

// expandedWritingTopics pins the topics added by migration 023. They follow
// public EJU 記述 prompts, 高考日语 opinion/chart tasks and 作文教材 task types.
var expandedWritingTopics = []struct {
	id    string
	level string
}{
	{"why-study-japanese", "N5"},
	{"one-day-diary", "N5"},
	{"hometown-seasons", "N5"},
	{"thanks-to-teacher", "N5"},
	{"decline-invitation", "N4"},
	{"new-year-card", "N4"},
	{"festival-guide", "N4"},
	{"recommend-a-product", "N4"},
	{"ebook-or-paper", "N3"},
	{"children-smartphone", "N3"},
	{"aging-chart", "N3"},
	{"learning-changed-me", "N3"},
	{"host-world-event", "N2"},
	{"long-university-break", "N2"},
	{"keigo-pros-cons", "N2"},
	{"self-driving-cars", "N2"},
	{"online-vs-classroom", "N2"},
	{"book-report", "N2"},
	{"population-decline-transport", "N1"},
	{"generative-ai-education", "N1"},
	{"aging-workforce", "N1"},
	{"environment-vs-growth", "N1"},
	{"cashless-society", "N1"},
}

var essayLengthHint = regexp.MustCompile(`[0-9]+～[0-9]+\s*(?:句|字)`)

func TestExpandedWritingTopics(t *testing.T) {
	topics := loadSeededWritingTopics(t)
	byID := make(map[string]seededWritingTopic, len(topics))
	for _, topic := range topics {
		byID[topic.ID] = topic
	}

	for _, tc := range expandedWritingTopics {
		t.Run(tc.id, func(t *testing.T) {
			topic, ok := byID[tc.id]
			if !ok {
				t.Fatalf("missing topic %q: migration 023 not applied?", tc.id)
			}
			if topic.Title == "" || topic.EssayPrompt == "" || topic.SentencePrompt == "" {
				t.Fatalf("incomplete topic: %+v", topic)
			}
			if topic.Level != tc.level {
				t.Errorf("level = %q, want %q", topic.Level, tc.level)
			}
			if topic.Position < 65 {
				t.Errorf("position = %d, want >= 65 (new topics append after 022)", topic.Position)
			}
			if !strings.Contains(topic.EssayPrompt, "用日语写") || !essayLengthHint.MatchString(topic.EssayPrompt) {
				t.Errorf("essay prompt lacks a Japanese length requirement: %q", topic.EssayPrompt)
			}
			if topic.EssayPrompt == topic.SentencePrompt {
				t.Error("essay prompt must differ from sentence prompt")
			}
			if !strings.Contains(topic.SentencePrompt, "「〜") {
				t.Errorf("sentence prompt lacks a target grammar pattern: %q", topic.SentencePrompt)
			}
			if !strings.Contains(topic.SentencePrompt, "1 句") &&
				!strings.Contains(topic.SentencePrompt, "1 个完整的日语句子") {
				t.Errorf("sentence prompt lacks the one-sentence requirement: %q", topic.SentencePrompt)
			}
		})
	}
}

// TestExpandedWritingTopicsCoverReferenceTaskTypes keeps the added topics
// honest: they must carry the task types found in the reference material
// instead of being more of the same proposal/notice prompts.
func TestExpandedWritingTopicsCoverReferenceTaskTypes(t *testing.T) {
	topics := loadSeededWritingTopics(t)
	expanded := make(map[string]bool, len(expandedWritingTopics))
	for _, tc := range expandedWritingTopics {
		expanded[tc.id] = true
	}

	cases := []struct {
		name    string
		keyword string
		min     int
	}{
		{"EJU 式双面立场意见文", "两种立场", 5},
		{"图表与数据说明", "图表", 2},
		{"议论文文体统一", "だ・である体", 2},
		{"初级文体统一", "です・ます体", 1},
		{"5W1H 信息传达", "5W1H", 1},
		{"反论回应", "回应", 4},
		{"具体例证", "具体", 10},
		{"三段结构", "三段", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			for _, topic := range topics {
				if expanded[topic.ID] && strings.Contains(topic.EssayPrompt, tc.keyword) {
					count++
				}
			}
			if count < tc.min {
				t.Errorf("only %d new topics mention %q, want at least %d", count, tc.keyword, tc.min)
			}
		})
	}
}

func TestWritingTopicCatalogIntegrity(t *testing.T) {
	topics := loadSeededWritingTopics(t)
	seenIDs := make(map[string]bool, len(topics))
	seenTitles := make(map[string]bool, len(topics))
	seenPositions := make(map[int]bool, len(topics))
	counts := make(map[string]int, len(topics))
	for _, topic := range topics {
		if seenIDs[topic.ID] {
			t.Errorf("duplicate topic id: %q", topic.ID)
		}
		if seenTitles[topic.Title] {
			t.Errorf("duplicate topic title: %q", topic.Title)
		}
		if seenPositions[topic.Position] {
			t.Errorf("duplicate topic position: %d", topic.Position)
		}
		seenIDs[topic.ID] = true
		seenTitles[topic.Title] = true
		seenPositions[topic.Position] = true
		counts[topic.Level]++
	}

	for _, tc := range []struct {
		level string
		min   int
	}{
		{"N5", 14}, {"N4", 16}, {"N3", 15}, {"N2", 30}, {"N1", 13},
	} {
		t.Run(tc.level, func(t *testing.T) {
			if counts[tc.level] < tc.min {
				t.Errorf("got %d topics, want at least %d", counts[tc.level], tc.min)
			}
		})
	}
}
