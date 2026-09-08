package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newMockAI serves a minimal OpenAI Responses API fixture and verifies that
// every active narration module marker is present in the privileged
// instructions field sent with each request.
func newMockAI(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Instructions string `json:"instructions"`
			Input        string `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		// Verify module wiring: privileged instructions must contain module
		// markers for every active module.
		for _, marker := range []string{"[F1]", "[V1]", "[C1]", "[P1]", "[L1]", "[R1]"} {
			if !strings.Contains(req.Instructions, marker) {
				t.Errorf("missing module marker %s in instructions", marker)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"output_text": "Spoken rewrite. Second sentence."})
	}))
	t.Cleanup(srv.Close)
	return srv
}
