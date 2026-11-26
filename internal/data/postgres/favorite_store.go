package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"japanese-learning-app/internal/module/favorite"
)

type FavoriteStore struct {
	db *sql.DB
}

func NewFavoriteStore(db *sql.DB) *FavoriteStore {
	return &FavoriteStore{db: db}
}

func (s *FavoriteStore) List(ctx context.Context, userID int64) ([]favorite.Item, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH saved AS (
			SELECT kind,target_key FROM content_favorites WHERE user_id=$1
			UNION ALL SELECT 'word',word_id::text FROM word_bookmarks WHERE user_id=$1
			UNION
			SELECT 'listening_sentence',p.lesson_id || ':' || b.position
			FROM listening_progress p CROSS JOIN LATERAL jsonb_array_elements_text(p.bookmarks_json) AS b(position)
			WHERE p.user_id=$1
		)
		SELECT s.kind,s.target_key,coalesce(c.title,''),coalesce(c.detail,''),coalesce(c.jlpt_level,''),coalesce(c.path,'')
		FROM saved s LEFT JOIN favorite_content c ON c.kind=s.kind AND c.target_key=s.target_key
			AND (c.owner_id IS NULL OR c.owner_id=$1)
		ORDER BY c.title NULLS LAST,s.kind,s.target_key`, userID)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}
	defer rows.Close()
	items := []favorite.Item{}
	for rows.Next() {
		var item favorite.Item
		if err := rows.Scan(&item.Kind, &item.Key, &item.Title, &item.Detail, &item.JLPTLevel, &item.Path); err != nil {
			return nil, fmt.Errorf("scan favorite: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read favorites: %w", err)
	}
	return items, nil
}

func (s *FavoriteStore) Set(ctx context.Context, userID int64, target favorite.Target, saved bool) (*favorite.Item, error) {
	var item *favorite.Item
	if saved {
		item = &favorite.Item{Target: target}
		err := s.db.QueryRowContext(ctx, `SELECT title,detail,jlpt_level,path
			FROM favorite_content WHERE kind=$2 AND target_key=$3 AND (owner_id IS NULL OR owner_id=$1)`,
			userID, target.Kind, target.Key).Scan(&item.Title, &item.Detail, &item.JLPTLevel, &item.Path)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, favorite.ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("load favorite target: %w", err)
		}
	}

	if !saved && target.Kind == "listening_sentence" {
		if err := s.removeListeningFavorite(ctx, userID, target); err != nil {
			return nil, err
		}
		return nil, nil
	}
	var err error
	switch target.Kind {
	case "word":
		if saved {
			_, err = s.db.ExecContext(ctx, `INSERT INTO word_bookmarks (user_id,word_id) VALUES ($1,$2::text::bigint) ON CONFLICT DO NOTHING`, userID, target.Key)
		} else {
			_, err = s.db.ExecContext(ctx, `DELETE FROM word_bookmarks WHERE user_id=$1 AND word_id=$2::text::bigint`, userID, target.Key)
		}

	default:
		if saved {
			_, err = s.db.ExecContext(ctx, `INSERT INTO content_favorites(user_id,kind,target_key) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, userID, target.Kind, target.Key)
		} else {
			_, err = s.db.ExecContext(ctx, `DELETE FROM content_favorites WHERE user_id=$1 AND kind=$2 AND target_key=$3`, userID, target.Kind, target.Key)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("save favorite %s: %w", target.Kind, err)
	}
	return item, nil
}

// Removing a sentence also removes any bookmark saved by the older listening UI.
func (s *FavoriteStore) removeListeningFavorite(ctx context.Context, userID int64, target favorite.Target) error {
	lessonID, position, _ := strings.Cut(target.Key, ":")
	return withQueryerTx(ctx, s.db, func(tx queryer) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM content_favorites WHERE user_id=$1 AND kind=$2 AND target_key=$3`, userID, target.Kind, target.Key); err != nil {
			return fmt.Errorf("delete listening favorite: %w", err)
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE listening_progress p SET bookmarks_json=coalesce((
				SELECT jsonb_agg(b.value ORDER BY b.ord)
				FROM jsonb_array_elements(p.bookmarks_json) WITH ORDINALITY AS b(value,ord)
				WHERE b.value <> to_jsonb($3::text::bigint)
			),'[]'::jsonb)
			WHERE user_id=$1 AND lesson_id=$2::text::bigint
			AND bookmarks_json @> jsonb_build_array($3::text::bigint)`, userID, lessonID, position)
		if err != nil {
			return fmt.Errorf("delete legacy listening bookmark: %w", err)
		}
		return nil
	})
}
