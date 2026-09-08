// Package ai implements the OpenAI Responses API adapter used for rewriting.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config for the rewrite AI client.
type Config struct {
	Provider   string // openrouter or openai
	APIKey     string // required at request time
	BaseURL    string // default https://api.openai.com/v1
	Model      string // required; no invented default
	Timeout    time.Duration
	MaxRetries int
	HTTPClient *http.Client
}

// HTTPError is a typed API error with retry classification.
type HTTPError struct {
	Status     int
	Body       string // truncated body; may contain API error info, never credentials
	Retryable  bool
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("ai: HTTP %d: %s", e.Status, e.Body)
}

// Client talks to the OpenAI Responses API.
type Client struct {
	cfg Config
	hc  *http.Client
}

// New builds a client. Validate must pass before any request.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("ai: missing API key; set NARRATE_AI_API_KEY (or OPENAI_API_KEY) or configure it in your config file")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("ai: no model configured; set NARRATE_AI_MODEL explicitly (a default model is not assumed)")
	}
	if cfg.Provider == "" {
		cfg.Provider = "openai"
	}
	if cfg.Provider != "openai" && cfg.Provider != "openrouter" {
		return nil, fmt.Errorf("ai: unsupported provider %q", cfg.Provider)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
		if cfg.Provider == "openrouter" {
			cfg.BaseURL = "https://openrouter.ai/api/v1"
		}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}
	return &Client{cfg: cfg, hc: hc}, nil
}

// Validate reports configuration problems without making billable calls.
func (c *Client) Validate() error {
	if c.cfg.APIKey == "" {
		return fmt.Errorf("ai: missing API key; see NARRATE_AI_API_KEY in README")
	}
	if c.cfg.Model == "" {
		return fmt.Errorf("ai: no model configured; set NARRATE_AI_MODEL (e.g. in config or environment)")
	}
	return nil
}

// Model returns the configured model identifier.
func (c *Client) Model() string { return c.cfg.Model }

// responsesRequest is the OpenAI Responses API payload.
type responsesRequest struct {
	Model           string `json:"model"`
	Instructions    string `json:"instructions"`
	Input           string `json:"input"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
}

type responsesResponse struct {
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	OutputText string `json:"output_text,omitempty"` // SDK convenience; not always present
	Error      *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Rewrite sends instructions (privileged) and prompt (contains source) to the
// Responses API. Bounded retries apply to 429/5xx and transient network errors.
func (c *Client) Rewrite(ctx context.Context, instructions, prompt string) (string, error) {
	var request any = responsesRequest{
		Model:           c.cfg.Model,
		Instructions:    instructions,
		Input:           prompt,
		MaxOutputTokens: 16384,
	}
	if c.cfg.Provider == "openrouter" {
		request = map[string]any{"model": c.cfg.Model, "messages": []map[string]string{{"role": "system", "content": instructions}, {"role": "user", "content": prompt}}}
	}
	reqBody, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	maxAttempts := c.cfg.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		out, retryable, retryAfter, err := c.attempt(ctx, reqBody)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !retryable || attempt == maxAttempts-1 {
			break
		}
		wait := retryAfter
		if wait == 0 {
			wait = backoffFor(attempt)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}
	return "", lastErr
}

func (c *Client) attempt(ctx context.Context, body []byte) (out string, retryable bool, retryAfter time.Duration, err error) {
	endpoint := "/responses"
	if c.cfg.Provider == "openrouter" {
		endpoint = "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+endpoint, bytes.NewReader(body))
	if err != nil {
		return "", false, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		// network error: retryable unless context canceled
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", false, 0, ctxErr
		}
		return "", true, 0, fmt.Errorf("ai: connection failed: %w", err)
	}
	defer resp.Body.Close()

	// Bound response size.
	limited := io.LimitReader(resp.Body, 8<<20)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", true, 0, fmt.Errorf("ai: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		status := resp.StatusCode
		ra, _ := time.ParseDuration(resp.Header.Get("Retry-After") + "s")
		retryable := status == 429 || status >= 500
		return "", retryable, ra, &HTTPError{
			Status:     status,
			Body:       truncate(strings.ReplaceAll(string(data), c.cfg.APIKey, "[redacted]"), 300),
			Retryable:  retryable,
			RetryAfter: ra,
		}
	}

	if c.cfg.Provider == "openrouter" {
		var rr struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(data, &rr); err != nil {
			return "", false, 0, fmt.Errorf("ai: malformed chat response: %w", err)
		}
		if len(rr.Error) > 0 && string(rr.Error) != "null" {
			return "", false, 0, fmt.Errorf("ai: provider returned an error")
		}
		if len(rr.Choices) == 0 || strings.TrimSpace(rr.Choices[0].Message.Content) == "" {
			return "", false, 0, fmt.Errorf("ai: response contained no output text")
		}
		if rr.Choices[0].FinishReason != "stop" {
			return "", false, 0, fmt.Errorf("ai: incomplete chat response (finish reason %q)", rr.Choices[0].FinishReason)
		}
		return rr.Choices[0].Message.Content, false, 0, nil
	}
	var rr responsesResponse
	if err := json.Unmarshal(data, &rr); err != nil {
		return "", false, 0, fmt.Errorf("ai: malformed response: %w", err)
	}
	if rr.Error != nil {
		return "", false, 0, fmt.Errorf("ai: API error (%s): %s", rr.Error.Type, strings.ReplaceAll(rr.Error.Message, c.cfg.APIKey, "[redacted]"))
	}
	if rr.OutputText != "" {
		return rr.OutputText, false, 0, nil
	}
	var b strings.Builder
	for _, o := range rr.Output {
		for _, cpart := range o.Content {
			if cpart.Type == "output_text" {
				b.WriteString(cpart.Text)
			}
		}
	}
	if b.Len() == 0 {
		return "", false, 0, fmt.Errorf("ai: response contained no output text")
	}
	return b.String(), false, 0, nil
}

func backoffFor(attempt int) time.Duration {
	d := 500 * time.Millisecond << uint(attempt)
	if d > 8*time.Second {
		d = 8 * time.Second
	}
	return d
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
