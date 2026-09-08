// Package narration implements chunking, prompt assembly, and the rewrite step.
package narration

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

//go:embed prompts/*.txt
var promptFS embed.FS

// PromptVersion is the human-readable version of the embedded instruction set.
const PromptVersion = "narrate-craft-v1"

// ModuleIDs lists the modules, in assembly order, before the style module.
var ModuleIDs = []string{"fidelity", "voice", "concreteness", "pacing", "listening"}

// StyleModules maps a style to its additional module.
var StyleModules = map[string]string{
	"conversational": "conversational",
	"coach":          "coach",
	"agent-update":   "agent-update",
}

// ReviewModuleID is included on both first attempts and repairs.
const ReviewModuleID = "review"

// ManifestEntry describes one embedded prompt module.
type ManifestEntry struct {
	ID     string `json:"id"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// PromptManifest records all embedded assets and their digests.
type PromptManifest struct {
	Version string          `json:"version"`
	Modules []ManifestEntry `json:"modules"`
}

// loadModule reads one module's exact bytes.
func loadModule(id string) ([]byte, error) {
	data, err := promptFS.ReadFile("prompts/" + id + ".txt")
	if err != nil {
		return nil, fmt.Errorf("narration: missing embedded prompt module %q: %w", id, err)
	}
	return data, nil
}

// ModulesForStyle returns the full ordered module set for a style.
func ModulesForStyle(style string) ([]string, error) {
	styleMod, ok := StyleModules[style]
	if !ok {
		return nil, fmt.Errorf("narration: unknown style %q", style)
	}
	return append(append([]string{}, ModuleIDs...), styleMod, ReviewModuleID), nil
}

// Assemble builds the privileged instruction text for one style.
func Assemble(style string) (string, error) {
	ids, err := ModulesForStyle(style)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, id := range ids {
		data, err := loadModule(id)
		if err != nil {
			return "", err
		}
		b.Write(data)
		if !strings.HasSuffix(string(data), "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n') // blank line between modules
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// EffectiveDigest hashes the exact ordered instruction bytes plus style.
// It is used for rewrite-cache identity; changing any active module invalidates
// cached rewrites.
func EffectiveDigest(style string) (string, error) {
	text, err := Assemble(style)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	io.WriteString(h, PromptVersion+"\x00")
	io.WriteString(h, style+"\x00")
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Manifest returns the prompt asset manifest for artifacts.
func Manifest() (PromptManifest, error) {
	m := PromptManifest{Version: PromptVersion}
	ids, err := ModulesForStyle("conversational")
	if err != nil {
		return m, err
	}
	// Include every additional style module in the artifact manifest.
	if _, err := ModulesForStyle("coach"); err != nil {
		return m, err
	}
	ids = append(ids, "coach", "agent-update")
	for _, id := range ids {
		data, err := loadModule(id)
		if err != nil {
			return m, err
		}
		sum := sha256.Sum256(data)
		m.Modules = append(m.Modules, ManifestEntry{
			ID:     id,
			File:   "prompts/" + id + ".txt",
			SHA256: hex.EncodeToString(sum[:]),
			Bytes:  len(data),
		})
	}
	return m, nil
}
