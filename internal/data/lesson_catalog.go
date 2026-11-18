package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"japanese-learning-app/internal/module/lesson"
)

type LessonCatalog struct {
	db    *sql.DB
	query string
}

func NewLessonCatalog(db *sql.DB) *LessonCatalog {
	tags := "array_to_json(tags)::text"
	matches := `strpos(lower(title), lower($2)) > 0 OR EXISTS (
 SELECT 1 FROM unnest(tags) AS tag WHERE strpos(lower(tag), lower($2)) > 0
 )`
	query := fmt.Sprintf(`WITH filtered AS (
		SELECT id, title, jlpt_level, %s AS tags, char_count
		FROM lessons WHERE speaking_material_id IS NULL
		AND ($1 = '' OR jlpt_level = $1) AND ($2 = '' OR %s)
	), page AS (
		SELECT * FROM filtered ORDER BY id LIMIT $3 OFFSET $4
	)
	SELECT p.id, p.title, p.jlpt_level, p.tags, p.char_count, totals.count
	FROM (SELECT count(*) AS count FROM filtered) AS totals
	LEFT JOIN page AS p ON true ORDER BY p.id`, tags, matches)
	return &LessonCatalog{db: db, query: query}
}

func (s *LessonCatalog) ListCatalog(ctx context.Context, query lesson.CatalogQuery) (lesson.CatalogPage, error) {
	page := lesson.CatalogPage{Items: []lesson.CatalogItem{}, Page: query.Page, Size: query.Size}
	offset := int64(query.Page-1) * int64(query.Size)
	rows, err := s.db.QueryContext(ctx, s.query, query.Level, query.Search, query.Size, offset)
	if err != nil {
		return page, fmt.Errorf("query lesson catalog: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, chars sql.NullInt64
		var title, level, tags sql.NullString
		if err := rows.Scan(&id, &title, &level, &tags, &chars, &page.Total); err != nil {
			return page, fmt.Errorf("scan lesson catalog: %w", err)
		}
		if !id.Valid {
			continue
		}
		item := lesson.CatalogItem{ID: id.Int64, Title: title.String, JLPTLevel: lesson.JLPTLevel(level.String), CharCount: int(chars.Int64)}
		if err := json.Unmarshal([]byte(tags.String), &item.Tags); err != nil {
			return page, fmt.Errorf("parse lesson %d catalog tags: %w", item.ID, err)
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("read lesson catalog: %w", err)
	}
	return page, nil
}
