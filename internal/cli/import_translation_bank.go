package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
)

// ── Bank file schema (言叶日课 / yanye-rike server/data/bank.json) ──────────

// TranslationBankFile is the top-level shape of an exported question bank.
type TranslationBankFile struct {
	Sentences []TranslationBankSentence         `json:"sentences"`
	Grammar   map[string]TranslationBankGrammar `json:"grammar"`
}

// TranslationBankSentence is one bilingual practice sentence.
type TranslationBankSentence struct {
	ID     string   `json:"id"`
	Level  string   `json:"level"`
	Topic  string   `json:"topic"`
	ZH     string   `json:"zh"`
	JA     string   `json:"ja"`
	Lesson string   `json:"lesson"`
	Book   string   `json:"book"`
	Origin string   `json:"origin"`
	Tags   []string `json:"tags"`
	Points []string `json:"points"`
}

// TranslationBankGrammar is a grammar point description keyed by pattern name.
type TranslationBankGrammar struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// ── Options / result ───────────────────────────────────────────────────────

// TranslationBankOptions tunes the import.
type TranslationBankOptions struct {
	// DefaultLevel is used for grammar points whose level cannot be inferred
	// from the sentences that reference them.
	DefaultLevel string
	// Replace drops sentences of an already-imported source before re-importing.
	Replace bool
	// ExamplesPerPoint caps how many bank sentences become grammar_examples.
	ExamplesPerPoint int
}

// TranslationBankResult reports what the import did.
type TranslationBankResult struct {
	Sources         int
	Sentences       int
	SkippedSources  int
	GrammarInserted int
	GrammarSkipped  int
	GrammarFallback int
	GrammarExamples int
	Levels          map[string]int
}

// ── Entry points ───────────────────────────────────────────────────────────

// ImportTranslationBankFile reads a bank JSON file and imports it into PostgreSQL.
func ImportTranslationBankFile(ctx context.Context, db *sql.DB, path string, opts TranslationBankOptions) (*TranslationBankResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cli.ImportTranslationBankFile read %s: %w", path, err)
	}

	var bank TranslationBankFile
	if err := json.Unmarshal(raw, &bank); err != nil {
		return nil, fmt.Errorf("cli.ImportTranslationBankFile unmarshal %s: %w", path, err)
	}

	return ImportTranslationBank(ctx, db, bank, opts)
}

// ImportTranslationBank imports a parsed bank into PostgreSQL.
func ImportTranslationBank(ctx context.Context, db *sql.DB, bank TranslationBankFile, opts TranslationBankOptions) (*TranslationBankResult, error) {
	if opts.DefaultLevel == "" {
		opts.DefaultLevel = "N3"
	}
	if !isJLPTLevel(opts.DefaultLevel) {
		return nil, fmt.Errorf("cli.ImportTranslationBank: invalid default level %q", opts.DefaultLevel)
	}
	if opts.ExamplesPerPoint <= 0 {
		opts.ExamplesPerPoint = 3
	}

	levels, err := groupSentencesByLevel(bank.Sentences)
	if err != nil {
		return nil, err
	}

	result := &TranslationBankResult{Levels: map[string]int{}}

	ordered := make([]string, 0, len(levels))
	for level := range levels {
		ordered = append(ordered, level)
	}
	sort.Strings(ordered)

	for _, level := range ordered {
		sentences := levels[level]
		title := fmt.Sprintf("言叶日课·%s翻译题库", level)

		sourceID, existed, err := ensureTranslationSource(ctx, db, title, opts.Replace)
		if err != nil {
			return nil, err
		}
		if existed && !opts.Replace {
			result.SkippedSources++
			slog.Info("cli.ImportTranslationBank: source already imported, skipped", "level", level, "source_id", sourceID)
			continue
		}

		n, err := insertBankSentences(ctx, db, sourceID, sentences)
		if err != nil {
			return nil, err
		}

		result.Sources++
		result.Sentences += n
		result.Levels[level] = n / 2
		slog.Info("cli.ImportTranslationBank: level imported", "level", level, "sentences", n)
	}

	plans, fallback := planBankGrammar(bank, opts.DefaultLevel)
	inserted, skipped, examples, err := importBankGrammar(ctx, db, plans, opts.ExamplesPerPoint)
	if err != nil {
		return nil, err
	}
	result.GrammarInserted = inserted
	result.GrammarSkipped = skipped
	result.GrammarFallback = fallback
	result.GrammarExamples = examples

	return result, nil
}

// ── Internals ──────────────────────────────────────────────────────────────

func groupSentencesByLevel(sentences []TranslationBankSentence) (map[string][]TranslationBankSentence, error) {
	levels := map[string][]TranslationBankSentence{}
	for _, s := range sentences {
		level := strings.ToUpper(strings.TrimSpace(s.Level))
		if !isJLPTLevel(level) {
			return nil, fmt.Errorf("cli.ImportTranslationBank: sentence %q has invalid level %q", s.ID, s.Level)
		}
		if strings.TrimSpace(s.ZH) == "" || strings.TrimSpace(s.JA) == "" {
			continue
		}
		levels[level] = append(levels[level], s)
	}
	return levels, nil
}

func isJLPTLevel(level string) bool {
	switch strings.ToUpper(level) {
	case "N1", "N2", "N3", "N4", "N5":
		return true
	}
	return false
}

// ensureTranslationSource returns the source id for title, creating it when missing.
// The second return value reports whether the source already existed.
func ensureTranslationSource(ctx context.Context, db *sql.DB, title string, replace bool) (int64, bool, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM translation_sources WHERE title = $1`, title).Scan(&id)
	switch {
	case err == nil:
		if replace {
			if _, err := db.ExecContext(ctx,
				`DELETE FROM translation_records
				 WHERE sentence_id IN (SELECT id FROM translation_sentences WHERE source_id = $1)`, id); err != nil {
				return 0, false, fmt.Errorf("cli.ensureTranslationSource delete records: %w", err)
			}
			if _, err := db.ExecContext(ctx, `DELETE FROM translation_sentences WHERE source_id = $1`, id); err != nil {
				return 0, false, fmt.Errorf("cli.ensureTranslationSource delete sentences: %w", err)
			}
		}
		return id, true, nil
	case err == sql.ErrNoRows:
		// fall through and create
	default:
		return 0, false, fmt.Errorf("cli.ensureTranslationSource lookup: %w", err)
	}

	err = db.QueryRowContext(ctx,
		`INSERT INTO translation_sources (title, source_type, source_url, api_endpoint, raw_content, origin)
		 VALUES ($1, 'manual', '', '', '', $2)
		 RETURNING id`,
		title,
		"yanye-rike",
	).Scan(&id)
	if err != nil {
		return 0, false, fmt.Errorf("cli.ensureTranslationSource insert: %w", err)
	}
	return id, false, nil
}

// sentenceBatchSize is the number of rows per multi-row INSERT. Batching keeps
// the number of network round-trips low when importing a full bank.
const sentenceBatchSize = 200

// insertBankSentences writes both directions (cn2jp, jp2cn) for every sentence.
func insertBankSentences(ctx context.Context, db *sql.DB, sourceID int64, sentences []TranslationBankSentence) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cli.insertBankSentences begin: %w", err)
	}
	defer tx.Rollback()

	written := 0
	batch := make([]bankSentenceRow, 0, sentenceBatchSize)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		n, err := execBankSentenceBatch(ctx, tx, batch)
		if err != nil {
			return err
		}
		written += n
		batch = batch[:0]
		return nil
	}

	for i, s := range sentences {
		level := strings.ToUpper(strings.TrimSpace(s.Level))
		tags := s.Tags
		if tags == nil {
			tags = []string{}
		}
		points := s.Points
		if points == nil {
			points = []string{}
		}

		batch = append(batch,
			bankSentenceRow{
				SourceID: sourceID, Direction: "cn2jp", SourceText: s.ZH, Reference: s.JA,
				Position: i * 2, Level: level, Topic: s.Topic, ExternalID: s.ID, Tags: tags, Points: points,
			},
			bankSentenceRow{
				SourceID: sourceID, Direction: "jp2cn", SourceText: s.JA, Reference: s.ZH,
				Position: i*2 + 1, Level: level, Topic: s.Topic, ExternalID: s.ID, Tags: tags, Points: points,
			},
		)

		if len(batch) >= sentenceBatchSize {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}

	if err := flush(); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cli.insertBankSentences commit: %w", err)
	}
	return written, nil
}

type bankSentenceRow struct {
	SourceID   int64
	Direction  string
	SourceText string
	Reference  string
	Position   int
	Level      string
	Topic      string
	ExternalID string
	Tags       []string
	Points     []string
}

func execBankSentenceBatch(ctx context.Context, tx *sql.Tx, rows []bankSentenceRow) (int, error) {
	const colsPerRow = 10

	var sb strings.Builder
	sb.WriteString(`INSERT INTO translation_sentences
		   (source_id, direction, source_text, reference_translation, position,
		    jlpt_level, topic, external_id, tags, points)
		 VALUES `)

	args := make([]any, 0, len(rows)*colsPerRow)
	for i, r := range rows {
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i * colsPerRow
		sb.WriteString("(")
		for c := 0; c < colsPerRow; c++ {
			if c > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "$%d", base+c+1)
		}
		sb.WriteString(")")

		args = append(args,
			r.SourceID,
			r.Direction,
			r.SourceText,
			r.Reference,
			r.Position,
			r.Level,
			r.Topic,
			r.ExternalID,
			r.Tags,
			r.Points,
		)
	}
	sb.WriteString(" ON CONFLICT (source_id, position) DO NOTHING")

	res, err := tx.ExecContext(ctx, sb.String(), args...)
	if err != nil {
		return 0, fmt.Errorf("cli.execBankSentenceBatch exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cli.execBankSentenceBatch rows affected: %w", err)
	}
	return int(affected), nil
}

// bankGrammarPlan is a grammar point ready to be written, with the example
// sentences that demonstrate it.
type bankGrammarPlan struct {
	Name     string
	Desc     string
	Level    string
	Examples []TranslationBankSentence
}

// planBankGrammar resolves a level and example sentences for every grammar entry.
// The second return value counts entries whose level had to fall back to the default.
func planBankGrammar(bank TranslationBankFile, defaultLevel string) ([]bankGrammarPlan, int) {
	levelOf := buildPointLevelIndex(bank.Sentences)
	examplesOf := buildExampleIndex(bank.Sentences)

	keys := make([]string, 0, len(bank.Grammar))
	for k := range bank.Grammar {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	plans := make([]bankGrammarPlan, 0, len(keys))
	fallback := 0

	for _, key := range keys {
		entry := bank.Grammar[key]
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			name = strings.TrimSpace(key)
		}
		desc := strings.TrimSpace(entry.Desc)
		if desc == "" {
			continue
		}

		level, ok := pointLevel(levelOf, key, name)
		if !ok {
			level = defaultLevel
			fallback++
		}

		plans = append(plans, bankGrammarPlan{
			Name:     name,
			Desc:     desc,
			Level:    level,
			Examples: exampleSentences(examplesOf, key, name),
		})
	}

	return plans, fallback
}

// importBankGrammar inserts the planned grammar points and their examples.
// Existing points are left untouched but still get examples filled in when the
// (grammar_point_id, position) slots are free.
func importBankGrammar(ctx context.Context, db *sql.DB, plans []bankGrammarPlan, examplesPerPoint int) (inserted, skipped, examples int, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("cli.importBankGrammar begin: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO grammar_points (name, meaning, conjunction_rule, usage_note, jlpt_level)
		 VALUES ($1, $2, '', '', $3)
		 ON CONFLICT (name, jlpt_level) DO NOTHING`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("cli.importBankGrammar prepare: %w", err)
	}
	defer stmt.Close()

	lookup, err := tx.PrepareContext(ctx,
		`SELECT id FROM grammar_points WHERE name = $1 AND jlpt_level = $2`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("cli.importBankGrammar prepare lookup: %w", err)
	}
	defer lookup.Close()

	exampleStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO grammar_examples (grammar_point_id, position, japanese, chinese)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (grammar_point_id, position) DO NOTHING`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("cli.importBankGrammar prepare examples: %w", err)
	}
	defer exampleStmt.Close()

	for _, plan := range plans {
		res, err := stmt.ExecContext(ctx, plan.Name, plan.Desc, plan.Level)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("cli.importBankGrammar exec %q: %w", plan.Name, err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return 0, 0, 0, fmt.Errorf("cli.importBankGrammar rows affected: %w", err)
		}
		if affected == 0 {
			skipped++
		} else {
			inserted++
		}

		var pointID int64
		if err := lookup.QueryRowContext(ctx, plan.Name, plan.Level).Scan(&pointID); err != nil {
			return 0, 0, 0, fmt.Errorf("cli.importBankGrammar lookup %q: %w", plan.Name, err)
		}

		limit := examplesPerPoint
		if limit > len(plan.Examples) {
			limit = len(plan.Examples)
		}
		for i := 0; i < limit; i++ {
			ex := plan.Examples[i]
			res, err := exampleStmt.ExecContext(ctx, pointID, i, ex.JA, ex.ZH)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("cli.importBankGrammar example %q: %w", plan.Name, err)
			}
			affected, err := res.RowsAffected()
			if err != nil {
				return 0, 0, 0, fmt.Errorf("cli.importBankGrammar example rows affected: %w", err)
			}
			examples += int(affected)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, 0, fmt.Errorf("cli.importBankGrammar commit: %w", err)
	}
	return inserted, skipped, examples, nil
}

// buildExampleIndex groups bank sentences by the grammar points they carry.
func buildExampleIndex(sentences []TranslationBankSentence) map[string][]TranslationBankSentence {
	index := map[string][]TranslationBankSentence{}
	seen := map[string]map[string]bool{}

	add := func(key string, s TranslationBankSentence) {
		key = normalizePointName(key)
		if key == "" {
			return
		}
		if seen[key] == nil {
			seen[key] = map[string]bool{}
		}
		if seen[key][s.ID] {
			return
		}
		seen[key][s.ID] = true
		index[key] = append(index[key], s)
	}

	for _, s := range sentences {
		if strings.TrimSpace(s.ZH) == "" || strings.TrimSpace(s.JA) == "" {
			continue
		}
		for _, p := range s.Points {
			add(p, s)
		}
		for _, tag := range s.Tags {
			add(tag, s)
		}
	}
	return index
}

// exampleSentences returns the sentences that demonstrate a grammar entry,
// trying the raw map key first and then the human readable name.
func exampleSentences(index map[string][]TranslationBankSentence, keys ...string) []TranslationBankSentence {
	for _, k := range keys {
		k = normalizePointName(k)
		if k == "" {
			continue
		}
		if sentences, ok := index[k]; ok && len(sentences) > 0 {
			return sentences
		}
	}
	return nil
}

var pointNameReplacer = strings.NewReplacer(
	"＋", "+",
	"（", "(",
	"）", ")",
	"〜", "",
	"~", "",
	" ", "",
	"\u3000", "",
)

// normalizePointName makes grammar point labels comparable across the slight
// typographic differences between the bank keys, names and sentence tags.
func normalizePointName(s string) string {
	return strings.ToLower(strings.TrimSpace(pointNameReplacer.Replace(s)))
}

// buildPointLevelIndex counts, for every grammar point name, how many sentences
// of each level reference it.
func buildPointLevelIndex(sentences []TranslationBankSentence) map[string]map[string]int {
	index := map[string]map[string]int{}
	for _, s := range sentences {
		level := strings.ToUpper(strings.TrimSpace(s.Level))
		for _, p := range s.Points {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if index[p] == nil {
				index[p] = map[string]int{}
			}
			index[p][level]++
		}
	}
	return index
}

// pointLevel resolves the level of a grammar entry, trying the map key first and
// then the human readable name.
func pointLevel(index map[string]map[string]int, keys ...string) (string, bool) {
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		counts, ok := index[k]
		if !ok {
			continue
		}
		best := ""
		bestCount := 0
		for level, n := range counts {
			if n > bestCount || (n == bestCount && level < best) {
				best, bestCount = level, n
			}
		}
		if best != "" {
			return best, true
		}
	}
	return "", false
}
