package translation_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"japanese-learning-app/internal/data"
	"japanese-learning-app/internal/module/translation"
	"japanese-learning-app/internal/module/user"
)

func TestTranslationPracticeSkipsSubmittedPrompts(t *testing.T) {
	db, err := data.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := data.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES
		(1, 'learner@example.com', 'unused'), (2, 'other@example.com', 'unused')`); err != nil {
		t.Fatal(err)
	}

	store := data.NewTranslationStore(db)
	addSource := func(title string) int64 {
		t.Helper()
		id, err := store.SaveSource(translation.TranslationSource{Title: title, SourceType: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	addSentence := func(sourceID int64, text, direction string, position int) int64 {
		t.Helper()
		id, err := store.SaveSentence(translation.TranslationSentence{
			SourceID: sourceID, Direction: direction, SourceText: text, Position: position,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	addRecord := func(userID, sentenceID int64, score int) {
		t.Helper()
		if _, err := store.SaveRecord(translation.TranslationRecord{
			UserID: userID, SentenceID: sentenceID, UserTranslation: "回答", Score: score,
		}); err != nil {
			t.Fatal(err)
		}
	}

	const jp2cn, cn2jp = "jp2cn", "cn2jp"
	first := addSource("素材一")
	second := addSource("素材二")
	low := addSentence(first, "次の駅です。", jp2cn, 1)
	high := addSentence(first, "おはようございます。", jp2cn, 2)
	zero := addSentence(first, "お待ちください。", jp2cn, 3)
	otherDirection := addSentence(first, "次の駅です。", cn2jp, 4)
	duplicateLow := addSentence(second, " 次の駅です。 ", jp2cn, 1)
	newPrompt := addSentence(second, "もう着きました。", jp2cn, 2)
	duplicateNew := addSentence(second, "もう着きました。", jp2cn, 3)
	addRecord(1, low, 65)
	addRecord(1, high, 100)
	addRecord(1, zero, 0)
	addRecord(2, newPrompt, 90)

	const secret = "translation-queue-test"
	mux := http.NewServeMux()
	translation.NewTranslationHandler(translation.NewTranslationService(store, nil)).RegisterRoutes(mux)
	handler := user.AuthMiddleware(secret, mux)
	const daily = "/api/v1/translation/queue?count=20"
	bySource := func(id int64) string {
		return "/api/v1/translation/sentences?source_id=" + strconv.FormatInt(id, 10)
	}
	for _, tc := range []struct {
		name   string
		userID int64
		path   string
		want   []int64
	}{
		{"daily ignores score and duplicate text", 1, daily, []int64{otherDirection, newPrompt}},
		{"free skips records across sources", 1, bySource(second) + "&unpracticed=true", []int64{newPrompt}},
		{"free preserves intentional review", 1, bySource(second), []int64{duplicateLow, newPrompt, duplicateNew}},
		{"another learner has separate progress", 2, daily, []int64{low, high, zero, otherDirection}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, _, err := user.SignToken(tc.userID, secret, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
			}
			var result struct {
				Data []translation.TranslationSentence `json:"data"`
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			got := make([]int64, 0, len(result.Data))
			for _, sentence := range result.Data {
				got = append(got, sentence.ID)
			}
			// 所有练习取题都乱序返回，因此按集合比较（want 均为升序）。
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("sentence IDs = %v, want %v", got, tc.want)
			}
		})
	}
}
