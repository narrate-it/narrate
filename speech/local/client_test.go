package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/narrate-it/narrate/speech/local"
)

func TestStreams(t *testing.T) {
	for _, tc := range []struct {
		name    string
		records []map[string]any
		want    error
	}{
		{"complete", nil, nil},
		{"truncated", []map[string]any{{"type": "format", "format": local.Format{Encoding: "pcm_s16le", SampleRate: 24000, Channels: 1}}}, local.ErrProtocol},
		{"stale", []map[string]any{{"type": "format", "generation": 99}}, local.ErrProtocol},
		{"out of order", []map[string]any{{"type": "format", "format": local.Format{Encoding: "pcm_s16le", SampleRate: 24000, Channels: 1}}, {"type": "frame", "sequence": 2, "samples": 1, "pcm": []byte{0, 0}}}, local.ErrProtocol},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("missing authorization")
				}
				var input map[string]any
				if json.NewDecoder(r.Body).Decode(&input) != nil {
					t.Error("request decode")
				}
				w.Header().Set("Content-Type", "application/x-ndjson")
				records := tc.records
				if records == nil {
					records = []map[string]any{{"type": "format", "format": local.Format{Encoding: "pcm_s16le", SampleRate: 24000, Channels: 1}}, {"type": "frame", "sequence": 1, "offset": 0, "samples": 2, "pcm": []byte{1, 0, 2, 0}}, {"type": "done", "sequence": 1, "offset": 2}}
				}
				for _, v := range records {
					if _, ok := v["generation"]; !ok {
						v["generation"] = input["generation"]
					}
					v["text_sequence"] = input["text_sequence"]
					if err := json.NewEncoder(w).Encode(v); err != nil {
						t.Error(err)
					}
				}
			}))
			defer s.Close()
			client, err := local.New(local.Config{Endpoint: s.URL, Token: "secret"})
			if err != nil {
				t.Fatal(err)
			}
			chunks := make(chan local.TextChunk, 2)
			chunks <- local.TextChunk{Sequence: 1, Text: "Hello."}
			chunks <- local.TextChunk{Sequence: 2, Text: "Goodbye."}
			close(chunks)
			var frames []local.Frame
			err = client.Synthesize(context.Background(), local.Request{Generation: 7, Voice: "anna", Chunks: chunks}, func(f local.Frame) error { frames = append(frames, f); return nil })
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if tc.want == nil {
				if len(frames) != 4 || frames[2].Offset != 2 || frames[3].Offset != 4 || !frames[1].PhraseEnd || frames[3].Sequence != 4 {
					t.Fatalf("bad frames: %+v", frames)
				}
			}
		})
	}
}
func TestCancellation(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
		close(exited)
	}))
	defer s.Close()
	c, err := local.New(local.Config{Endpoint: s.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	chunks := make(chan local.TextChunk, 1)
	chunks <- local.TextChunk{Sequence: 1, Text: "Hello"}
	result := make(chan error, 1)
	go func() {
		result <- c.Synthesize(ctx, local.Request{Generation: 1, Voice: "anna", Chunks: chunks}, func(local.Frame) error { return nil })
	}()
	<-entered
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation blocked")
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("HTTP request still running")
	}
}
func TestCallbackFailureStops(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, err := w.Write([]byte("{\"type\":\"format\",\"generation\":1,\"text_sequence\":1,\"format\":{\"encoding\":\"pcm_s16le\",\"sample_rate\":24000,\"channels\":1}}\n{\"type\":\"frame\",\"generation\":1,\"text_sequence\":1,\"sequence\":1,\"samples\":1,\"pcm\":\"AAA=\"}\n"))
		if err != nil {
			t.Error(err)
		}
	}))
	defer s.Close()
	c, err := local.New(local.Config{Endpoint: s.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	chunks := make(chan local.TextChunk, 1)
	chunks <- local.TextChunk{Sequence: 1, Text: "Hello"}
	close(chunks)
	stop := errors.New("player stopped")
	if err = c.Synthesize(context.Background(), local.Request{Generation: 1, Voice: "anna", Chunks: chunks}, func(local.Frame) error { return stop }); !errors.Is(err, stop) {
		t.Fatal(err)
	}
}
