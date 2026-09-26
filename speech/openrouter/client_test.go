package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplicitCredentialAndSingleCall(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer voice-test" {
			t.Error("wrong key")
		}
		w.WriteHeader(429)
		w.Write([]byte("secret body"))
	}))
	defer srv.Close()
	c := Client{Key: "voice-test", BaseURL: srv.URL}
	_, err := c.Speak(context.Background(), "model", "voice", "Hello")
	if err == nil || calls != 1 || strings.Contains(err.Error(), "secret") {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	c.Key = ""
	_, err = c.Speak(context.Background(), "model", "voice", "Hello")
	if err == nil || calls != 1 {
		t.Fatal("missing key called provider")
	}
}
func TestSpeechContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		if json.NewDecoder(r.Body).Decode(&b) != nil {
			t.Fatal("decode")
		}
		if b["response_format"] != "mp3" || b["input"] != "Hello" {
			t.Errorf("body=%v", b)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("X-Generation-Id", "gen-tts-test")
		w.Write([]byte("audio"))
	}))
	defer srv.Close()
	out, err := (Client{Key: "test", BaseURL: srv.URL}).Speak(context.Background(), "model", "voice", "Hello")
	if err != nil || string(out.Audio) != "audio" || out.GenerationID != "gen-tts-test" {
		t.Fatalf("out=%v err=%v", out, err)
	}
}
