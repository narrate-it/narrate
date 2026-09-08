package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRewriteSendsExactModuleBytes verifies the privileged instruction field
// carries the assembled modules and source stays in the input field.
func TestRewriteSendsExactModuleBytes(t *testing.T) {
	var gotInstructions, gotInput, gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path %q", r.URL.Path)
		}
		var req responsesRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotInstructions, gotInput, gotModel = req.Instructions, req.Input, req.Model
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"output_text": "ok script"})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", Model: "test-model", BaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Rewrite(context.Background(), "INSTRUCTIONS-HERE", "SOURCE-HERE")
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok script" {
		t.Fatalf("out %q", out)
	}
	if gotInstructions != "INSTRUCTIONS-HERE" || gotInput != "SOURCE-HERE" {
		t.Fatalf("field placement wrong: instr=%q input=%q", gotInstructions, gotInput)
	}
	if gotModel != "test-model" {
		t.Fatalf("model %q", gotModel)
	}
}

func TestRetryOn429(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(429)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"output_text": "fine"})
	}))
	defer srv.Close()

	c, _ := New(Config{APIKey: "k", Model: "m", BaseURL: srv.URL, HTTPClient: srv.Client(), MaxRetries: 1})
	out, err := c.Rewrite(context.Background(), "i", "p")
	if err != nil || out != "fine" || calls != 2 {
		t.Fatalf("out=%q err=%v calls=%d", out, err, calls)
	}
}

func TestAuthFailsPromptly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	c, _ := New(Config{APIKey: "bad", Model: "m", BaseURL: srv.URL, HTTPClient: srv.Client()})
	_, err := c.Rewrite(context.Background(), "i", "p")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected prompt auth failure, got %v", err)
	}
}

func TestRequiresExplicitModel(t *testing.T) {
	if _, err := New(Config{APIKey: "k"}); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestOpenRouterChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("wrong endpoint: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("missing authorization")
		}
		var body struct {
			Model    string                           `json:"model"`
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "provider/model" || len(body.Messages) != 2 {
			t.Errorf("wrong request: %+v", body)
		} else if body.Messages[0].Role != "system" || body.Messages[0].Content != "craft" || body.Messages[1].Content != "source" {
			t.Errorf("wrong messages: %+v", body.Messages)
		}
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"Spoken script."}}]}`))
	}))
	defer srv.Close()
	c, err := New(Config{Provider: "openrouter", APIKey: "test-secret", Model: "provider/model", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Rewrite(context.Background(), "craft", "source")
	if err != nil || out != "Spoken script." {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
