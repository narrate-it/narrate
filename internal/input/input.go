// Package input resolves CLI text sources: positional args, files, or stdin,
// with strict UTF-8 and binary-content rejection.
package input

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ErrAmbiguous is returned when both a file and positional text are supplied.
var ErrAmbiguous = errors.New("input: provide either -f FILE or text arguments, not both")

// Source is the resolved input text.
type Source struct {
	Text      string
	FromStdin bool
	FromFile  string
}

// Resolve implements the precedence contract:
//   - explicit file (-f, including "-" for stdin) wins;
//   - a single positional file path reads that file;
//   - else positional args joined with spaces;
//   - else stdin (whole document until EOF; for a terminal this means the
//     user must end with EOF, a documented difference from `say`).
func Resolve(file string, args []string, stdin io.Reader, stdinIsTerminal bool) (Source, error) {
	var s Source
	if file == "" && len(args) == 1 {
		candidate := args[0]
		if strings.HasPrefix(candidate, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return s, err
			}
			candidate = filepath.Join(home, candidate[2:])
		}
		_, err := os.Stat(candidate)
		if err == nil || !os.IsNotExist(err) || looksLikePath(candidate) {
			file = candidate
			args = nil
		}
	}
	switch {
	case file != "":
		if len(args) > 0 {
			return s, ErrAmbiguous
		}
		if file == "-" {
			data, err := io.ReadAll(stdin)
			if err != nil {
				return s, fmt.Errorf("input: reading stdin: %w", err)
			}
			s.FromStdin = true
			s.Text = string(data)
		} else {
			data, err := os.ReadFile(file)
			if err != nil {
				return s, fmt.Errorf("input: %w", err)
			}
			s.FromFile = file
			s.Text = string(data)
		}
	case len(args) > 0:
		s.Text = joinArgs(args)
	default:
		if stdinIsTerminal {
			return s, fmt.Errorf("input: no text given and stdin is a terminal; pass text, use -f FILE, or pipe input (end with Ctrl-D)")
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return s, fmt.Errorf("input: reading stdin: %w", err)
		}
		s.FromStdin = true
		s.Text = string(data)
	}
	if err := Validate(s.Text); err != nil {
		return s, err
	}
	return s, nil
}

// joinArgs joins positional arguments with single spaces, like `say`.
func joinArgs(args []string) string {
	out := ""
	for _, a := range args {
		if out == "" {
			out = a
		} else {
			out += " " + a
		}
	}
	return out
}

// Validate rejects empty input, invalid UTF-8, and obvious binary content.
func Validate(text string) error {
	if len(strings.TrimSpace(text)) == 0 {
		return errors.New("input: no text provided")
	}
	if !utf8.ValidString(text) {
		return errors.New("input: input is not valid UTF-8; convert the file (e.g. iconv -t UTF-8) and retry")
	}
	// Binary sniff: NUL bytes or a high proportion of control characters.
	control := 0
	for _, r := range text {
		if r == 0 {
			return errors.New("input: input contains NUL bytes and looks binary; PDFs and other binary formats are not supported (extract text first)")
		}
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			control++
		}
	}
	if control*100 > len(text)/2 {
		return errors.New("input: input contains many control characters and looks binary; text input only is supported")
	}
	return nil
}

// Recognize explicit local paths and common document names even when missing.
// Literal text resembling a path can always be piped through stdin.
func looksLikePath(value string) bool {
	if filepath.IsAbs(value) || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		return true
	}
	if strings.ContainsAny(value, " \t\n") || strings.Contains(value, "://") {
		return false
	}
	if strings.ContainsRune(value, filepath.Separator) {
		return true
	}
	switch strings.ToLower(filepath.Ext(value)) {
	case ".md", ".txt", ".markdown", ".rst", ".org", ".csv", ".json", ".yaml", ".yml", ".html", ".pdf", ".docx":
		return true
	}
	return false
}
