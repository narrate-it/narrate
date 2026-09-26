// Package narration exposes Narrate's embedded voice craft and script rewriting
// to applications. It performs no environment lookup, credential discovery,
// playback, or network work unless the supplied RewriteClient does so.
package narration

import internal "github.com/narrate-it/narrate/internal/narration"

const PromptVersion = internal.PromptVersion

type Chunk = internal.Chunk
type Chunker = internal.Chunker
type RewriteClient = internal.RewriteClient
type Rewriter = internal.Rewriter
type ManifestEntry = internal.ManifestEntry
type PromptManifest = internal.PromptManifest

var ErrTruncated = internal.ErrTruncated

// NewChunker creates a structure-aware chunker with bounded adjacent context.
func NewChunker(maxRunes, contextRunes int) Chunker {
	return internal.NewChunker(maxRunes, contextRunes)
}

// Assemble returns the canonical craft instructions for conversational, coach,
// or agent-update style. Source material belongs in a separate untrusted input.
func Assemble(style string) (string, error) { return internal.Assemble(style) }

// ModulesForStyle returns the ordered craft module identifiers.
func ModulesForStyle(style string) ([]string, error) { return internal.ModulesForStyle(style) }

// EffectiveDigest identifies the exact instructions used for a style.
func EffectiveDigest(style string) (string, error) { return internal.EffectiveDigest(style) }

// Manifest returns the version and digests of the embedded prompt assets.
func Manifest() (PromptManifest, error) { return internal.Manifest() }
