package data

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"japanese-learning-app/internal/module/lesson"
)

func TestLessonCatalog(t *testing.T) {
	db, err := OpenDB(t.TempDir() + "/catalog.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lessons (id, title, jlpt_level, tags_json) VALUES
		(1, '私の一日', 'N5', '["日常生活"]'),
		(2, '週末の計画', 'N4', '["生活"]'),
		(3, 'お互いさま', 'N2', '["人际关系"]'),
		(4, 'A 100% 日記', 'N2', '["生活"]'),
		(5, 'A 1000 日記', 'N2', '["生活"]')`); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	lesson.RegisterCatalogRoute(mux, NewLessonCatalog(db, false))
	for _, tt := range []struct {
		name, query               string
		status, total, page, size int
		ids                       []int64
	}{
		{"everything", "", 200, 5, 1, 20, []int64{1, 2, 3, 4, 5}},
		{"second page", "?level=N2&page=2&size=2", 200, 3, 2, 2, []int64{5}},
		{"filter by tag", "?level=N2&q=%E7%94%9F%E6%B4%BB&size=1", 200, 2, 1, 1, []int64{4}},
		{"global case-insensitive title with literal percent", "?q=a+100%25", 200, 1, 1, 20, []int64{4}},
		{"empty level", "?level=N1", 200, 0, 1, 20, []int64{}},
		{"page outside results", "?page=9", 200, 5, 9, 20, []int64{}},
		{"invalid level", "?level=N9", 400, 0, 0, 0, nil},
		{"invalid page", "?page=-1", 400, 0, 0, 0, nil},
		{"oversized page", "?size=101", 400, 0, 0, 0, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/lessons/catalog"+tt.query, nil))
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.status, rec.Body.String())
			}
			if tt.status != http.StatusOK {
				return
			}
			var result struct{ Data lesson.CatalogPage }
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			got := result.Data
			ids := make([]int64, 0, len(got.Items))
			for _, item := range got.Items {
				ids = append(ids, item.ID)
			}
			if got.Items == nil || !reflect.DeepEqual(ids, tt.ids) || got.Total != tt.total || got.Page != tt.page || got.Size != tt.size {
				t.Fatalf("catalog = %+v, IDs %v, want total=%d page=%d size=%d IDs=%v", got, ids, tt.total, tt.page, tt.size, tt.ids)
			}
		})
	}
}
