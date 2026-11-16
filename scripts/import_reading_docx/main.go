// Command import_reading_docx converts the vertical-Japanese reading-material
// collection .docx into the lesson JSON consumed by import-lessons-postgres.
//
// It deliberately does not talk to PostgreSQL: generating and validating the
// import file is a separate step from writing to the database.
package main

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

const (
	nsWord    = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsDrawing = "http://schemas.openxmlformats.org/drawingml/2006/main"
)

var (
	numberedHeadingRE = regexp.MustCompile(`^(\d{3})\s`)
	chapterHeadingRE  = regexp.MustCompile(`^第\s*\d+\s*篇\s*(\d{4}-\d{2}-\d{2})?\s*(.*)$`)
	urlRE             = regexp.MustCompile(`https?://[^\s]+`)
)

var allowedLevels = map[string]bool{"N5": true, "N4": true, "N3": true, "N2": true, "N1": true}

type para struct {
	style string
	text  string
	imgs  int
}

type article struct {
	number          int
	numberedHeading string
	date            string
	subtitle        string
	jp              []string
	cn              []string
	sourceURL       string
	translationURL  string
	images          int
}

type alignedSentence struct {
	Japanese string
	Chinese  string
}

// chineseOnlyArticle holds an article whose Japanese source exists only as a
// screenshot in the source document. It is kept as a separate archive file so
// the Chinese translations are not silently lost.
type chineseOnlyArticle struct {
	Number            int      `json:"docx_number"`
	Heading           string   `json:"docx_heading"`
	Date              string   `json:"source_date"`
	Title             string   `json:"title"`
	ChineseParagraphs []string `json:"chinese_paragraphs"`
	SourceURL         string   `json:"source_url"`
	TranslationURL    string   `json:"translation_url"`
	Reason            string   `json:"reason"`
}

type furiganaToken struct {
	Surface string `json:"surface"`
	Reading string `json:"reading"`
}

type sentenceOut struct {
	Index     int             `json:"index"`
	Tokens    []furiganaToken `json:"tokens"`
	Chinese   string          `json:"chinese"`
	StartMS   int64           `json:"start_ms"`
	EndMS     int64           `json:"end_ms"`
	Paragraph int             `json:"paragraph"`
}

type lesson struct {
	Title            string         `json:"title"`
	JLPTLevel        string         `json:"jlpt_level"`
	Tags             []string       `json:"tags"`
	AudioURL         string         `json:"audio_url"`
	VideoURL         string         `json:"video_url"`
	WordIDs          []int64        `json:"word_ids"`
	ShadowingEnabled bool           `json:"shadowing_enabled"`
	ShadowingVersion int            `json:"shadowing_version"`
	ShadowingConfig  map[string]any `json:"shadowing_config"`
	Sentences        []sentenceOut  `json:"sentences"`
}

func main() {
	docx := flag.String("docx", "", "path to the source .docx file")
	out := flag.String("out", "", "path to write the lesson import JSON")
	report := flag.String("report", "", "path to write the dry-run report")
	level := flag.String("level", "N2", "JLPT level for every generated lesson")
	limit := flag.Int("limit", 0, "only convert the first N articles (0 = all)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err := run(*docx, *out, *report, *level, *limit); err != nil {
		slog.Error("import_reading_docx failed", "err", err)
		os.Exit(1)
	}
}

func run(docxPath, outPath, reportPath, level string, limit int) error {
	if docxPath == "" || outPath == "" {
		return fmt.Errorf("-docx and -out are required")
	}
	if !allowedLevels[level] {
		return fmt.Errorf("invalid -level %q: %w", level, errInvalidLevel)
	}

	slog.Debug("reading docx", "docx", docxPath)
	paras, err := readDocxParagraphs(docxPath)
	if err != nil {
		return fmt.Errorf("read docx %q: %w", docxPath, err)
	}
	slog.Debug("docx paragraphs read", "count", len(paras))

	articles := splitArticles(paras)
	if limit > 0 && limit < len(articles) {
		articles = articles[:limit]
	}
	slog.Debug("articles split", "count", len(articles))

	tok, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return fmt.Errorf("create kagome tokenizer: %w", err)
	}

	lessons, chineseOnly, stats := buildLessons(articles, level, tok)
	if err := validateLessons(lessons); err != nil {
		return fmt.Errorf("validate generated lessons: %w", err)
	}
	slog.Debug("lessons validated", "lessons", len(lessons), "sentences", stats.sentences, "tokens", stats.tokens,
		"chinese_only", len(chineseOnly))

	raw, err := json.Marshal(lessons)
	if err != nil {
		return fmt.Errorf("marshal lessons: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(outPath, raw, 0o644); err != nil {
		return fmt.Errorf("write lesson JSON %q: %w", outPath, err)
	}

	archivePath := filepath.Join(filepath.Dir(outPath), "chinese_only_articles.json")
	archive, err := json.MarshalIndent(chineseOnly, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal chinese-only archive: %w", err)
	}
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		return fmt.Errorf("write chinese-only archive %q: %w", archivePath, err)
	}

	summary := buildReport(docxPath, outPath, archivePath, level, articles, stats)
	if reportPath != "" {
		if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
			return fmt.Errorf("create report dir: %w", err)
		}
		if err := os.WriteFile(reportPath, []byte(summary), 0o644); err != nil {
			return fmt.Errorf("write report %q: %w", reportPath, err)
		}
	}
	fmt.Print(summary)
	return nil
}

var errInvalidLevel = fmt.Errorf("level must be one of N5/N4/N3/N2/N1")

// readDocxParagraphs streams word/document.xml so the 280MB of embedded
// screenshots never have to be loaded into memory.
func readDocxParagraphs(docxPath string) ([]para, error) {
	zr, err := zip.OpenReader(docxPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return nil, fmt.Errorf("word/document.xml not found")
	}
	rc, err := docFile.Open()
	if err != nil {
		return nil, fmt.Errorf("open word/document.xml: %w", err)
	}
	defer rc.Close()

	dec := xml.NewDecoder(rc)
	var out []para
	var cur *para
	inText := false
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode word/document.xml: %w", err)
		}
		switch v := token.(type) {
		case xml.StartElement:
			switch {
			case v.Name.Space == nsWord && v.Name.Local == "p":
				cur = &para{}
			case cur == nil:
				continue
			case v.Name.Space == nsWord && v.Name.Local == "pStyle":
				for _, a := range v.Attr {
					if a.Name.Local == "val" {
						cur.style = a.Value
					}
				}
			case v.Name.Space == nsWord && v.Name.Local == "t":
				inText = true
			case v.Name.Space == nsDrawing && v.Name.Local == "blip":
				cur.imgs++
			}
		case xml.CharData:
			if inText && cur != nil {
				cur.text += string(v)
			}
		case xml.EndElement:
			switch {
			case v.Name.Space == nsWord && v.Name.Local == "t":
				inText = false
			case v.Name.Space == nsWord && v.Name.Local == "p" && cur != nil:
				out = append(out, *cur)
				cur = nil
			}
		}
	}
	return out, nil
}

func splitArticles(paras []para) []article {
	var articles []article
	var cur *article
	mode := ""
	for _, p := range paras {
		text := strings.TrimSpace(p.text)
		if p.style == "Heading1" {
			if m := numberedHeadingRE.FindStringSubmatch(text); m != nil {
				n, _ := strconv.Atoi(m[1])
				articles = append(articles, article{number: n, numberedHeading: text})
				cur = &articles[len(articles)-1]
				mode = ""
				continue
			}
			if cur != nil && strings.HasPrefix(text, "第 ") {
				date, title := parseChapterHeading(text)
				cur.date = date
				cur.subtitle = title
				if cur.subtitle == "" {
					cur.subtitle = subtitleFromNumbered(cur.numberedHeading)
				}
				continue
			}
		}
		if cur == nil {
			continue // cover page / front matter
		}
		if p.style == "Heading2" {
			switch {
			case strings.Contains(text, "日文"):
				mode = "jp"
			case strings.Contains(text, "中文"):
				mode = "cn"
			}
			continue
		}
		if p.imgs > 0 {
			cur.images += p.imgs
		}
		switch {
		case strings.HasPrefix(text, "原文来源"):
			if u := urlRE.FindString(text); u != "" {
				cur.sourceURL = u
			}
		case strings.HasPrefix(text, "翻译依据"):
			if u := urlRE.FindString(text); u != "" {
				cur.translationURL = u
			}
		case text == "":
			// image-only paragraph
		case mode == "jp":
			cur.jp = append(cur.jp, text)
		case mode == "cn":
			cur.cn = append(cur.cn, text)
		}
	}
	return articles
}

func parseChapterHeading(heading string) (string, string) {
	m := chapterHeadingRE.FindStringSubmatch(strings.TrimSpace(heading))
	if m == nil {
		return "", ""
	}
	return m[1], strings.TrimSpace(m[2])
}

func subtitleFromNumbered(heading string) string {
	if i := strings.Index(heading, "｜"); i >= 0 {
		return strings.TrimSpace(heading[i+len("｜"):])
	}
	return ""
}

func buildTitle(number int, date, subtitle string) string {
	base := fmt.Sprintf("日语阅读素材 %03d", number)
	switch {
	case subtitle != "":
		return base + "｜" + subtitle
	case date != "":
		return base + "｜" + date
	default:
		return base
	}
}

// alignParagraphs pairs Japanese and Chinese paragraphs by position. Surplus
// Chinese text is appended to the last Japanese paragraph so no translation
// content is silently dropped; missing translations stay empty.
func alignParagraphs(jp, cn []string) []alignedSentence {
	out := make([]alignedSentence, len(jp))
	for i := range jp {
		out[i] = alignedSentence{Japanese: jp[i]}
		if i < len(cn) {
			out[i].Chinese = cn[i]
		}
	}
	if len(cn) > len(jp) && len(out) > 0 {
		extra := strings.Join(cn[len(jp):], "\n")
		if out[len(out)-1].Chinese == "" {
			out[len(out)-1].Chinese = extra
		} else {
			out[len(out)-1].Chinese += "\n" + extra
		}
	}
	return out
}

type buildStats struct {
	sentences      int
	tokens         int
	readings       int
	misaligned     []int
	missingSource  []int
	skippedNoText  []int
	imagesExcluded int
}

func buildLessons(articles []article, level string, tok *tokenizer.Tokenizer) ([]lesson, []chineseOnlyArticle, buildStats) {
	lessons := make([]lesson, 0, len(articles))
	chineseOnly := make([]chineseOnlyArticle, 0)
	var stats buildStats
	for _, a := range articles {
		stats.imagesExcluded += a.images
		if a.sourceURL == "" {
			stats.missingSource = append(stats.missingSource, a.number)
		}
		if len(a.jp) == 0 {
			stats.skippedNoText = append(stats.skippedNoText, a.number)
			chineseOnly = append(chineseOnly, chineseOnlyArticle{
				Number:            a.number,
				Heading:           a.numberedHeading,
				Date:              a.date,
				Title:             buildTitle(a.number, a.date, a.subtitle),
				ChineseParagraphs: a.cn,
				SourceURL:         a.sourceURL,
				TranslationURL:    a.translationURL,
				Reason:            "日文原文仅存在于文档截图中，未生成课文；中文译文完整留档",
			})
			continue
		}
		if len(a.jp) != len(a.cn) {
			stats.misaligned = append(stats.misaligned, a.number)
		}

		aligned := alignParagraphs(a.jp, a.cn)
		sentences := make([]sentenceOut, 0, len(aligned))
		for i, s := range aligned {
			tokens := tokenizeText(tok, s.Japanese)
			stats.sentences++
			stats.tokens += len(tokens)
			for _, t := range tokens {
				if t.Reading != "" {
					stats.readings++
				}
			}
			sentences = append(sentences, sentenceOut{
				Index:     i,
				Tokens:    tokens,
				Chinese:   s.Chinese,
				Paragraph: i,
			})
		}

		tags := []string{"日语阅读素材", "网页译文回查版"}
		if a.date != "" {
			tags = append(tags, a.date)
		}
		lessons = append(lessons, lesson{
			Title:     buildTitle(a.number, a.date, a.subtitle),
			JLPTLevel: level,
			Tags:      tags,
			WordIDs:   []int64{},
			ShadowingConfig: map[string]any{
				"source":          "竖排日语文章合集 网页译文回查版",
				"docx_number":     a.number,
				"docx_heading":    a.numberedHeading,
				"source_date":     a.date,
				"source_url":      a.sourceURL,
				"translation_url": a.translationURL,
			},
			Sentences: sentences,
		})
	}
	return lessons, chineseOnly, stats
}

func tokenizeText(t *tokenizer.Tokenizer, text string) []furiganaToken {
	out := make([]furiganaToken, 0, 16)
	for _, tok := range t.Tokenize(text) {
		surface := tok.Surface
		if surface == "" {
			continue
		}
		reading := ""
		if containsKanji(surface) {
			if r, ok := tok.Reading(); ok && r != "" && r != "*" {
				reading = katakanaToHiragana(r)
			}
		}
		out = append(out, furiganaToken{Surface: surface, Reading: reading})
	}
	return out
}

func containsKanji(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han) {
			return true
		}
	}
	return false
}

func katakanaToHiragana(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0x30A1 && r <= 0x30F6 {
			return r - 0x60
		}
		return r
	}, s)
}

func validateLessons(lessons []lesson) error {
	if len(lessons) == 0 {
		return fmt.Errorf("no lessons generated")
	}
	seen := make(map[string]bool, len(lessons))
	for i, l := range lessons {
		if strings.TrimSpace(l.Title) == "" {
			return fmt.Errorf("lesson %d: empty title", i)
		}
		if !allowedLevels[l.JLPTLevel] {
			return fmt.Errorf("lesson %d (%s): invalid jlpt_level %q", i, l.Title, l.JLPTLevel)
		}
		key := l.Title + "|" + l.JLPTLevel
		if seen[key] {
			return fmt.Errorf("duplicate lesson title/level %q", key)
		}
		seen[key] = true
		if len(l.Sentences) == 0 {
			return fmt.Errorf("lesson %d (%s): no sentences", i, l.Title)
		}
		for j, s := range l.Sentences {
			if s.Index != j {
				return fmt.Errorf("lesson %d (%s) sentence %d: index = %d, want %d", i, l.Title, j, s.Index, j)
			}
			if len(s.Tokens) == 0 {
				return fmt.Errorf("lesson %d (%s) sentence %d: empty tokens", i, l.Title, j)
			}
			if s.EndMS < s.StartMS {
				return fmt.Errorf("lesson %d (%s) sentence %d: end_ms %d < start_ms %d", i, l.Title, j, s.EndMS, s.StartMS)
			}
			var text strings.Builder
			for _, t := range s.Tokens {
				text.WriteString(t.Surface)
			}
			if strings.TrimSpace(text.String()) == "" {
				return fmt.Errorf("lesson %d (%s) sentence %d: empty japanese text", i, l.Title, j)
			}
		}
	}
	return nil
}

func buildReport(docxPath, outPath, archivePath, level string, articles []article, stats buildStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 阅读素材导入计划（dry-run）\n\n")
	fmt.Fprintf(&b, "- 源文档：`%s`\n", docxPath)
	fmt.Fprintf(&b, "- 输出 JSON：`%s`\n", outPath)
	fmt.Fprintf(&b, "- 目标等级：%s\n", level)
	fmt.Fprintf(&b, "- 文档文章数：%d\n", len(articles))
	fmt.Fprintf(&b, "- 生成课文数：%d\n", len(articles)-len(stats.skippedNoText))
	fmt.Fprintf(&b, "- 生成句子（日文段）数：%d\n", stats.sentences)
	fmt.Fprintf(&b, "- 分词 token 数：%d（其中带假名读音 %d）\n", stats.tokens, stats.readings)
	fmt.Fprintf(&b, "- 日文/中文段落数不一致：%d 篇（多余中文并入末段，缺失中文留空，需人工回查）\n", len(stats.misaligned))
	fmt.Fprintf(&b, "- 缺少原文来源 URL：%d 篇\n", len(stats.missingSource))
	fmt.Fprintf(&b, "- 日文原文仅在截图中、未生成课文：%d 篇（中文译文已完整留档：`%s`）\n", len(stats.skippedNoText), archivePath)
	fmt.Fprintf(&b, "- 未入库的截图：%d 张（按用户确认，只导入文本）\n", stats.imagesExcluded)
	fmt.Fprintf(&b, "- 校验：通过（标题唯一、等级合法、句子索引连续、tokens 非空）\n\n")

	if len(stats.misaligned) > 0 {
		fmt.Fprintf(&b, "## 段落数不一致的篇号\n\n%s\n\n", joinInts(stats.misaligned))
	}
	if len(stats.missingSource) > 0 {
		fmt.Fprintf(&b, "## 缺少来源 URL 的篇号\n\n%s\n\n", joinInts(stats.missingSource))
	}
	if len(stats.skippedNoText) > 0 {
		fmt.Fprintf(&b, "## 无正文被跳过的篇号\n\n%s\n\n", joinInts(stats.skippedNoText))
	}
	fmt.Fprintf(&b, "## 未入库内容\n\n封面说明文字（非阅读素材）与全部网页截图按方案排除；每篇的日期、原文来源 URL、翻译依据 URL 已写入 `shadowing_config` 元数据。\n无日文正文的 %d 篇，其日文原文只存在于截图里，按方案不入库；中文译文完整保存在 `%s`，可后续补录日文后再导入。\n", len(stats.skippedNoText), archivePath)
	return b.String()
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ", ")
}
