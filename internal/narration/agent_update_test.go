package narration

import (
	"context"
	"encoding/json"
	"example.com/narrate/internal/ai"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentUpdateAllowsShortScriptWithoutWeakeningDocumentGuard(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		instructions, _ := body["instructions"].(string)
		if !strings.Contains(instructions, "[F1]") || !strings.Contains(instructions, "[A1]") {
			t.Error("missing fidelity or agent module")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"output_text":"Partial: 12 tests passed. Deployment remains unverified."}`))
	}))
	defer server.Close()
	client, err := ai.New(ai.Config{APIKey: "test", Model: "test", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	chunk := Chunk{Text: strings.Repeat("A source fact. ", 100), First: true, Last: true}
	parts, err := (&Rewriter{Client: client, Style: "agent-update"}).RewriteAll(context.Background(), []Chunk{chunk}, nil)
	if err != nil || len(parts) != 1 || calls != 1 {
		t.Fatalf("parts=%v calls=%d err=%v", parts, calls, err)
	}
	if !looksTruncated(parts[0], chunk) {
		t.Fatal("document guard was weakened")
	}
	document, _ := EffectiveDigest("conversational")
	agent, _ := EffectiveDigest("agent-update")
	if document == agent {
		t.Fatal("cache identity does not distinguish style")
	}
	manifest, err := Manifest()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range manifest.Modules {
		if m.ID == "agent-update" {
			found = true
		}
	}
	if !found {
		t.Fatal("manifest omits active module")
	}
}
