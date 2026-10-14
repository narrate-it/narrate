// Package local streams authenticated PCM from a persistent local Pocket service.
// It never starts processes, reads credentials from the environment, or retries.
package local

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrProtocol      = errors.New("local speech: invalid or incomplete stream")
	ErrAuthorization = errors.New("local speech: unauthorized")
	ErrOverload      = errors.New("local speech: overloaded")
	ErrUnavailable   = errors.New("local speech: unavailable")
)

const MaxTextBytes = 16 << 10

type Config struct {
	Endpoint, Token string
	HTTPClient      *http.Client
	PhraseTimeout   time.Duration
}
type Client struct {
	endpoint, token string
	http            *http.Client
	timeout         time.Duration
}
type TextChunk struct {
	Sequence uint64
	Text     string
}
type Request struct {
	Generation uint64
	Voice      string
	Chunks     <-chan TextChunk
}
type Format struct {
	Encoding   string `json:"encoding"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
}

// PhraseEnd marks an exact synthesized phrase boundary, not word alignment or playback.
// Boundary frames have zero Samples and carry the same monotonically increasing Sequence.
type Frame struct {
	Generation, Sequence, TextSequence, Offset uint64
	Samples                                    int
	PCM                                        []byte
	Format                                     Format
	PhraseEnd                                  bool
}
type record struct {
	Type         string `json:"type"`
	Generation   uint64 `json:"generation"`
	TextSequence uint64 `json:"text_sequence"`
	Sequence     uint64 `json:"sequence"`
	Offset       uint64 `json:"offset"`
	Samples      int    `json:"samples"`
	PCM          []byte `json:"pcm"`
	Format       Format `json:"format"`
}

func New(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("local speech: explicit service origin required")
	}
	if strings.TrimSpace(cfg.Token) == "" || strings.ContainsAny(cfg.Token, "\r\n") {
		return nil, errors.New("local speech: explicit bearer credential required")
	}
	if cfg.PhraseTimeout == 0 {
		cfg.PhraseTimeout = 45 * time.Second
	}
	if cfg.PhraseTimeout < 0 {
		return nil, errors.New("local speech: invalid timeout")
	}
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{}
	}
	copyHTTP := *h
	copyHTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{strings.TrimRight(cfg.Endpoint, "/"), cfg.Token, &copyHTTP, cfg.PhraseTimeout}, nil
}

// Synthesize processes stable chunks in order with bounded memory and callback backpressure.
// Closing Chunks means successful input completion. Cancellation or any stream failure returns
// an error; an HTTP EOF without a validated done record can never mean success.
func (c *Client) Synthesize(ctx context.Context, r Request, emit func(Frame) error) error {
	if r.Generation == 0 || r.Voice != "anna" || r.Chunks == nil || emit == nil {
		return errors.New("local speech: generation, Anna voice, chunks and callback required")
	}
	var textSequence, sequence, offset uint64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-r.Chunks:
			if !ok {
				return ctx.Err()
			}
			if chunk.Sequence != textSequence+1 || len(chunk.Text) > MaxTextBytes || strings.TrimSpace(chunk.Text) == "" {
				return fmt.Errorf("%w: text chunk", ErrProtocol)
			}
			textSequence = chunk.Sequence
			if err := c.phrase(ctx, r, chunk, &sequence, &offset, emit); err != nil {
				return err
			}
		}
	}
}
func (c *Client) phrase(ctx context.Context, r Request, t TextChunk, seq, offset *uint64, emit func(Frame) error) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	body, _ := json.Marshal(struct {
		Generation uint64 `json:"generation"`
		Sequence   uint64 `json:"text_sequence"`
		Voice      string `json:"voice"`
		Text       string `json:"text"`
	}{r.Generation, t.Sequence, r.Voice, t.Text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/synthesize", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: transport", ErrUnavailable)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 401, 403:
		return ErrAuthorization
	case 429, 503:
		return ErrOverload
	default:
		return ErrUnavailable
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/x-ndjson") {
		return ErrProtocol
	}
	scan := bufio.NewScanner(io.LimitReader(resp.Body, 12<<20))
	scan.Buffer(make([]byte, 4096), 128<<10)
	format := Format{}
	var localSequence, localOffset uint64
	done := false
	for scan.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var v record
		if json.Unmarshal(scan.Bytes(), &v) != nil || v.Generation != r.Generation || v.TextSequence != t.Sequence || done {
			return ErrProtocol
		}
		switch v.Type {
		case "format":
			if format.SampleRate != 0 || localSequence != 0 || v.Format != (Format{"pcm_s16le", 24000, 1}) {
				return ErrProtocol
			}
			format = v.Format
		case "frame":
			if format.SampleRate == 0 || v.Sequence != localSequence+1 || v.Offset != localOffset || v.Samples <= 0 || v.Samples > 4800 || len(v.PCM) != v.Samples*2 {
				return ErrProtocol
			}
			localSequence = v.Sequence
			localOffset += uint64(v.Samples)
			*seq++
			if err := emit(Frame{Generation: r.Generation, Sequence: *seq, TextSequence: t.Sequence, Offset: *offset, Samples: v.Samples, PCM: v.PCM, Format: format}); err != nil {
				return err
			}
			*offset += uint64(v.Samples)
		case "done":
			if format.SampleRate == 0 || localOffset == 0 || v.Sequence != localSequence || v.Offset != localOffset {
				return ErrProtocol
			}
			done = true
		default:
			return ErrProtocol
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if scan.Err() != nil || !done {
		return ErrProtocol
	}
	*seq++
	return emit(Frame{Generation: r.Generation, Sequence: *seq, TextSequence: t.Sequence, Offset: *offset, Format: format, PhraseEnd: true})
}
