package postgres

import (
	"context"
	"fmt"

	"japanese-learning-app/internal/module/word"
)

func (s *WordStore) Search(ctx context.Context, query string, limit int) ([]word.SearchHit, error) {
	// These paired alphabets fold katakana to hiragana in the term and readings.
	const katakana = "ァアィイゥウェエォオカガキギクグケゲコゴサザシジスズセゼソゾタダチヂッツヅテデトドナニヌネノハバパヒビピフブプヘベペホボポマミムメモャヤュユョヨラリルレロヮワヰヱヲンヴヵヶヽヾ"
	const hiragana = "ぁあぃいぅうぇえぉおかがきぎくぐけげこごさざしじすずせぜそぞただちぢっつづてでとどなにぬねのはばぱひびぴふぶぷへべぺほぼぽまみむめもゃやゅゆょよらりるれろゎわゐゑをんゔゕゖゝゞ"
	rows, err := s.db.QueryContext(ctx, `
		WITH term AS (
		    SELECT lower(normalize($1, NFKC)) AS literal
		), hits AS (
		    SELECT w.id, w.kanji_form, w.reading, w.meaning, w.part_of_speech, w.jlpt_level,
		           CASE
		               WHEN form = literal THEN 0
		               WHEN kana_form = kana_term OR kana_reading = kana_term THEN 1
		               WHEN starts_with(kana_form, kana_term) OR starts_with(kana_reading, kana_term) THEN 2
		               WHEN strpos(kana_form, kana_term) > 0 OR strpos(kana_reading, kana_term) > 0 THEN 3
		               WHEN meaning_text = literal THEN 4
		               WHEN strpos(meaning_text, literal) > 0 THEN 5
		           END AS rank
		    FROM words w CROSS JOIN term
		    CROSS JOIN LATERAL (
		        SELECT lower(normalize(w.kanji_form, NFKC)) AS form,
		               lower(normalize(w.reading, NFKC)) AS reading_text,
		               lower(normalize(w.meaning, NFKC)) AS meaning_text
		    ) normalized
		    CROSS JOIN LATERAL (
		        SELECT translate(form, $2, $3) AS kana_form,
		               translate(reading_text, $2, $3) AS kana_reading,
		               translate(literal, $2, $3) AS kana_term
		    ) kana
		)
		SELECT id, kanji_form, reading, meaning, part_of_speech, jlpt_level
		FROM hits WHERE rank IS NOT NULL
		ORDER BY rank, length(kanji_form), id
		LIMIT $4`, query, katakana, hiragana, limit)
	if err != nil {
		return nil, fmt.Errorf("query dictionary words: %w", translateError(err))
	}
	defer rows.Close()

	hits := []word.SearchHit{}
	for rows.Next() {
		var hit word.SearchHit
		if err := rows.Scan(&hit.ID, &hit.KanjiForm, &hit.Reading, &hit.Meaning, &hit.PartOfSpeech, &hit.JLPTLevel); err != nil {
			return nil, fmt.Errorf("scan dictionary word: %w", err)
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read dictionary words: %w", err)
	}
	return hits, nil
}
