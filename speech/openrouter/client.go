// Package openrouter implements bounded, single-attempt OpenRouter speech calls.
// Credentials are explicit; it never reads ambient environment or CLI config.
package openrouter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"
)

const BaseURL = "https://openrouter.ai/api/v1"
const MaxAudioBytes = 2 << 20
const MaxSpeechBytes = 4 << 20

type Client struct {
	Key     string
	HTTP    *http.Client
	BaseURL string
}
type Result struct {
	Text         string
	Audio        []byte
	MediaType    string
	GenerationID string
	Usage        json.RawMessage
}

// CallError deliberately excludes provider bodies, credentials and input text.
// MayHaveExecuted requires callers to preserve possible billing on uncertainty.
type CallError struct {
	Status          int
	MayHaveExecuted bool
	Reason          string
}

func (e *CallError) Error() string { return "speech: " + e.Reason }
func (c Client) call(ctx context.Context, path string, body any, max int64) (Result, []byte, error) {
	var out Result
	if strings.TrimSpace(c.Key) == "" {
		return out, nil, &CallError{Reason: "dedicated voice credential required"}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return out, nil, err
	}
	base := c.BaseURL
	if base == "" {
		base = BaseURL
	}
	if base != BaseURL {
		u, err := url.Parse(base)
		if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return out, nil, errors.New("speech: unsupported endpoint")
		}
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+path, bytes.NewReader(raw))
	if err != nil {
		return out, nil, errors.New("speech: invalid request")
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	sent := false
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { sent = true }}))
	hc := http.Client{Timeout: 60 * time.Second}
	if c.HTTP != nil {
		hc = *c.HTTP
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := hc.Do(req)
	if err != nil {
		return out, nil, &CallError{MayHaveExecuted: sent, Reason: "request interrupted; billing may be unknown"}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return out, nil, &CallError{Status: resp.StatusCode, MayHaveExecuted: true, Reason: "provider refused speech request"}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil || int64(len(data)) > max {
		return out, nil, &CallError{MayHaveExecuted: true, Reason: "invalid or oversized response"}
	}
	out.MediaType = resp.Header.Get("Content-Type")
	out.GenerationID = resp.Header.Get("X-Generation-Id")
	if len(out.GenerationID) > 256 {
		out.GenerationID = ""
	}
	return out, data, nil
}
func (c Client) Transcribe(ctx context.Context, model, language string, wav []byte) (Result, error) {
	if len(wav) < 44 || len(wav) > MaxAudioBytes || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return Result{}, errors.New("speech: bounded WAV audio required")
	}
	body := map[string]any{"model": model, "input_audio": map[string]string{"data": base64.StdEncoding.EncodeToString(wav), "format": "wav"}, "response_format": "json"}
	if language != "" {
		body["language"] = language
	}
	out, data, err := c.call(ctx, "/audio/transcriptions", body, 64<<10)
	if err != nil {
		return out, err
	}
	var wire struct {
		Text  string          `json:"text"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &wire) != nil || len(wire.Text) > 8192 {
		return out, &CallError{MayHaveExecuted: true, Reason: "invalid transcript"}
	}
	out.Text = strings.TrimSpace(wire.Text)
	out.Usage = wire.Usage
	return out, nil
}
func (c Client) Speak(ctx context.Context, model, voice, text string) (Result, error) {
	if strings.TrimSpace(text) == "" || len([]rune(text)) > 4000 || model == "" || voice == "" {
		return Result{}, errors.New("speech: model, voice and bounded text required")
	}
	out, data, err := c.call(ctx, "/audio/speech", map[string]any{"model": model, "voice": voice, "input": text, "response_format": "mp3"}, MaxSpeechBytes)
	if err != nil {
		return out, err
	}
	if !strings.HasPrefix(out.MediaType, "audio/") || len(data) == 0 {
		return out, &CallError{MayHaveExecuted: true, Reason: "invalid speech audio"}
	}
	out.Audio = data
	return out, nil
}

// Rewrite implements narration.RewriteClient. The host admits this call separately
// from synthesis. The fixed small text model does not receive tools or history.
func (c Client) Rewrite(ctx context.Context, instructions, source string) (string, error) {
	out, data, err := c.call(ctx, "/chat/completions", map[string]any{"model": "google/gemini-2.5-flash-lite", "max_tokens": 1200, "messages": []map[string]string{{"role": "system", "content": instructions}, {"role": "user", "content": source}}}, 64<<10)
	_ = out
	if err != nil {
		return "", err
	}
	var wire struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &wire) != nil || len(wire.Choices) != 1 || wire.Choices[0].FinishReason != "stop" {
		return "", &CallError{MayHaveExecuted: true, Reason: "incomplete narration"}
	}
	return wire.Choices[0].Message.Content, nil
}
