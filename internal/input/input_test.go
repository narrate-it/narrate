package input

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPositionalJoin(t *testing.T) {
	s, err := Resolve("", []string{"hello", "there"}, strings.NewReader(""), true)
	if err != nil || s.Text != "hello there" {
		t.Fatalf("s=%+v err=%v", s, err)
	}
}

func TestAmbiguous(t *testing.T) {
	_, err := Resolve("f.txt", []string{"x"}, strings.NewReader(""), true)
	if err != ErrAmbiguous {
		t.Fatalf("want ErrAmbiguous, got %v", err)
	}
}

func TestStdinDash(t *testing.T) {
	s, err := Resolve("-", nil, strings.NewReader("piped"), true)
	if err != nil || s.Text != "piped" || !s.FromStdin {
		t.Fatalf("s=%+v err=%v", s, err)
	}
}

func TestEmptyRejected(t *testing.T) {
	if _, err := Resolve("", nil, strings.NewReader(" "), false); err == nil {
		t.Fatal("expected empty rejection")
	}
}

func TestBinaryRejected(t *testing.T) {
	if _, err := Resolve("", []string{"bad\x00doc"}, nil, true); err == nil {
		t.Fatal("expected binary rejection")
	}
}

func TestInvalidUTF8(t *testing.T) {
	if _, err := Resolve("", []string{string([]byte{0xff, 0xfe})}, nil, true); err == nil {
		t.Fatal("expected invalid-utf8 rejection")
	}
}

func TestPositionalFileReadsContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report with spaces.md")
	text := "The pilot lasts seven days. Stop if the error rate exceeds five percent."
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	src, err := Resolve("", []string{path}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if src.Text != text || src.FromFile != path {
		t.Fatalf("file not read: %+v", src)
	}
}

func TestMissingPositionalPathFails(t *testing.T) {
	for _, path := range []string{filepath.Join(t.TempDir(), "missing.md"), "./missing", "missing-report.md"} {
		if _, err := Resolve("", []string{path}, nil, true); err == nil {
			t.Errorf("missing file treated as text: %s", path)
		}
	}
}

func TestPositionalDirectoryFails(t *testing.T) {
	if _, err := Resolve("", []string{t.TempDir()}, nil, true); err == nil {
		t.Fatal("directory treated as text")
	}
}
