package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorWithDetails(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteErrorWithDetails(rec, http.StatusConflict, "ERR_SHADOWING_VERSION_STALE", "shadowing version is stale", "", map[string]any{
		"current_shadowing_version": 2,
	})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	var got APIError
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.Code != "ERR_SHADOWING_VERSION_STALE" {
		t.Fatalf("code = %q, want %q", got.Code, "ERR_SHADOWING_VERSION_STALE")
	}

	value, ok := got.Details["current_shadowing_version"]
	if !ok {
		t.Fatalf("details missing current_shadowing_version: %#v", got.Details)
	}
	if value != float64(2) {
		t.Fatalf("current_shadowing_version = %#v, want %v", value, float64(2))
	}
}
