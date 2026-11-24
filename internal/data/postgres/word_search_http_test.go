package postgres

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"japanese-learning-app/internal/module/user"
	"japanese-learning-app/internal/module/word"
)

func TestWordDictionarySearch(t *testing.T) {
	db := newMigratedPostgresTestDB(t)
	store := newPostgresWordStoreForDB(db)
	entries := []word.Word{
		{KanjiForm: "準備", Reading: "じゅんび", Meaning: "准备", PartOfSpeech: "名词", JLPTLevel: word.LevelN5},
		{KanjiForm: "準備運動", Reading: "じゅんびうんどう", Meaning: "热身运动", JLPTLevel: word.LevelN2},
		{KanjiForm: "予習", Reading: "よしゅう", Meaning: "课前准备", JLPTLevel: word.LevelN3},
		{KanjiForm: "ブログ", Reading: "ブログ", Meaning: "博客", JLPTLevel: word.LevelN2},
		{KanjiForm: "100%", Reading: "ひゃくパーセント", Meaning: "百分之百", JLPTLevel: word.LevelN1},
	}
	for i, entry := range entries {
		id, err := store.InsertWord(entry)
		if err != nil {
			t.Fatal(err)
		}
		entries[i].ID = id
	}
	protected := http.NewServeMux()
	word.NewWordHandler(word.NewWordService(store)).RegisterRoutes(protected)
	mux := user.AuthMiddleware("dictionary-test", protected)
	token, _, err := user.SignToken(1, "dictionary-test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, query string
		want        []int64
		status      int
	}{
		{"exact before prefix across levels", " 準備 ", []int64{entries[0].ID, entries[1].ID}, 200},
		{"reading exact before prefix", "ジュンビ", []int64{entries[0].ID, entries[1].ID}, 200},
		{"hiragana query matches katakana", "ぶろぐ", []int64{entries[3].ID}, 200},
		{"half-width query matches full-width kana", "ﾌﾞﾛｸﾞ", []int64{entries[3].ID}, 200},
		{"Chinese meaning exact before partial", "准备", []int64{entries[0].ID, entries[2].ID}, 200},
		{"percent is literal", "%", []int64{entries[4].ID}, 200},
		{"no matches", "この単語は辞書にない", nil, 200},
		{"blank query", "　 ", nil, 400},
		{"overlong query", strings.Repeat("語", 101), nil, 400},
		{"invalid text", "\x00", nil, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/words/search?q="+url.QueryEscape(tt.query), nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.status, rec.Body.String())
			}
			if tt.status != http.StatusOK {
				return
			}
			var envelope struct {
				Data []struct{ ID int64 } `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Data == nil {
				t.Fatal("search hits must be a JSON array")
			}
			var got []int64
			for _, hit := range envelope.Data {
				got = append(got, hit.ID)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("IDs = %v, want %v", got, tt.want)
			}
		})
	}

	var records int
	if err := db.QueryRow(`SELECT count(*) FROM word_records`).Scan(&records); err != nil || records != 0 {
		t.Fatalf("learning records = %d, error = %v", records, err)
	}
}
