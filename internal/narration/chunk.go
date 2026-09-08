package narration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Chunk is one structure-aware piece of the source document.
type Chunk struct {
	Index int    // stable, 0-based, in source order
	ID    string // stable identifier derived from index + content hash
	Text  string // content to rewrite (exclusive of context)
	Prev  string // short adjacent context before, not to be re-narrated
	Next  string // short adjacent context after, not to be re-narrated
	First bool
	Last  bool
}

// Chunker splits documents into bounded, structure-aware chunks.
type Chunker struct {
	MaxRunes int // maximum runes per chunk
	CtxRunes int // adjacent-context runes included around each chunk
}

// NewChunker returns a chunker with limits sized for typical model request windows.
func NewChunker(maxRunes, ctxRunes int) Chunker {
	if maxRunes <= 0 {
		maxRunes = 12000
	}
	if ctxRunes < 0 {
		ctxRunes = 0
	}
	if ctxRunes > 500 {
		ctxRunes = 500
	}
	return Chunker{MaxRunes: maxRunes, CtxRunes: ctxRunes}
}

// paragraphs splits on blank lines, preserving single newlines within a paragraph.
func paragraphs(text string) []string {
	parts := strings.Split(text, "\n\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, p)
	}
	return out
}

// chunkParagraph splits one oversized paragraph on sentence boundaries, then
// falls back to rune windows so a single giant paragraph still fits.
func chunkParagraph(p string, maxRunes int) []string {
	if len([]rune(p)) <= maxRunes {
		return []string{p}
	}
	sentences := splitSentences(p)
	var chunks []string
	cur := strings.Builder{}
	for _, s := range sentences {
		if cur.Len() > 0 && cur.Len()+len([]rune(s))+1 > maxRunes {
			chunks = append(chunks, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
		// A single sentence longer than maxRunes is hard-split into windows.
		if len([]rune(s)) > maxRunes {
			if cur.Len() > 0 {
				chunks = append(chunks, strings.TrimSpace(cur.String()))
				cur.Reset()
			}
			rs := []rune(s)
			for len(rs) > maxRunes {
				// prefer splitting at a space
				cut := maxRunes
				for cut > maxRunes/2 && cut < len(rs) && rs[cut] != ' ' {
					cut--
				}
				if cut == maxRunes/2 {
					// no space found
					cut = maxRunes
				}
				chunks = append(chunks, strings.TrimRight(string(rs[:cut]), " "))
				rs = rs[cut:]
			}
			cur.WriteString(string(rs))
			continue
		}
		cur.WriteString(s)
		cur.WriteString(" ")
	}
	if cur.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(cur.String()))
	}
	return chunks
}

// splitSentences does conservative sentence splitting on ., !, ? followed by
// whitespace. It is heuristic but only used for oversized-paragraph fallback.
func splitSentences(p string) []string {
	var out []string
	start := 0
	rs := []rune(p)
	for i := 1; i < len(rs)-1; i++ {
		if (rs[i] == '.' || rs[i] == '!' || rs[i] == '?') && (rs[i+1] == ' ' || rs[i+1] == '\n') {
			out = append(out, string(rs[start:i+1]))
			start = i + 1
		}
	}
	out = append(out, string(rs[start:]))
	return out
}

// Chunk splits text into ordered chunks with stable IDs and adjacent context.
func (c Chunker) Chunk(text string) []Chunk {
	paras := paragraphs(text)
	// Build pieces: either whole paragraphs or sub-split oversized ones.
	var pieces []string
	for _, p := range paras {
		pieces = append(pieces, chunkParagraph(p, c.MaxRunes)...)
	}
	// Merge adjacent small pieces up to MaxRunes to reduce chunk count.
	var merged []string
	cur := ""
	for _, p := range pieces {
		if cur != "" && len([]rune(cur))+len([]rune(p))+2 <= c.MaxRunes {
			cur += "\n\n" + p
		} else {
			if cur != "" {
				merged = append(merged, cur)
			}
			cur = p
		}
	}
	if cur != "" {
		merged = append(merged, cur)
	}
	chunks := make([]Chunk, len(merged))
	for i, t := range merged {
		sum := sha256.Sum256([]byte(t))
		chunks[i] = Chunk{
			Index: i,
			ID:    fmt.Sprintf("c%03d-%s", i, hex.EncodeToString(sum[:6])),
			Text:  t,
			First: i == 0,
			Last:  i == len(merged)-1,
		}
		if i > 0 {
			chunks[i].Prev = tailRunes(merged[i-1], c.CtxRunes)
		}
		if i < len(merged)-1 {
			chunks[i].Next = headRunes(merged[i+1], c.CtxRunes)
		}
	}
	return chunks
}

func headRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}

func tailRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[len(rs)-n:])
}

// JoinScript concatenates rewritten chunk scripts with blank lines.
func JoinScript(parts []string) string {
	return strings.Join(parts, "\n\n")
}
