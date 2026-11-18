package errata_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestAudioErrataCannotBypassWithdrawalRestriction(t *testing.T) {
	for _, category := range []string{"other", "prompt", "answer", "explanation"} {
		t.Run(category, func(t *testing.T) {
			e := newEditEnv(t)
			id := e.insert(t, 1, "speaking", "42", "audio", "audio", "bad audio", "pending")
			path := fmt.Sprintf("/api/v1/errata/%d", id)
			e.request(t, http.MethodPatch, path, e.token1, fmt.Sprintf(`{"category":%q}`, category), http.StatusConflict)
			e.request(t, http.MethodDelete, path, e.token1, "", http.StatusConflict)
		})
	}
}
