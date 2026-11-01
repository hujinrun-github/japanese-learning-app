package notion

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientQueryDataSourceUsesNotionHeadersAndPagination(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/data_sources/source/query" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Notion-Version"); got != "2025-09-03" {
			t.Fatalf("Notion-Version = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			_, _ = w.Write([]byte(`{"results":[{"id":"page-1","url":"https://notion.so/page-1"}],"has_more":true,"next_cursor":"cursor-1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"id":"page-2","url":"https://notion.so/page-2"}],"has_more":false}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	pages, err := client.QueryDataSource("source")
	if err != nil {
		t.Fatalf("QueryDataSource failed: %v", err)
	}
	if len(pages) != 2 || pages[0].ID != "page-1" || pages[1].ID != "page-2" {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestClientRejectsEmptyToken(t *testing.T) {
	if _, err := NewClientChecked("", ""); err == nil {
		t.Fatal("expected empty token error")
	}
}

func TestClientRetriesRateLimit(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"slow down"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[],"has_more":false}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.URL)
	client.retrySleep = func(time.Duration) {}
	if err := client.TestDataSource("source"); err != nil {
		t.Fatalf("TestDataSource failed after retry: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}
