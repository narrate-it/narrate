//go:build darwin

package cli

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPublishTempClearsHiddenFlag(t *testing.T) {
	for _, force := range []bool{true, false} {
		dir := t.TempDir()
		tmp := filepath.Join(dir, ".narrate-audio-test")
		if err := os.WriteFile(tmp, []byte("audio"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Chflags(tmp, ufHidden); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(dir, "out.mp3")
		if err := publishTemp(tmp, dest, force); err != nil {
			t.Fatalf("force=%v: %v", force, err)
		}
		var st syscall.Stat_t
		if err := syscall.Stat(dest, &st); err != nil {
			t.Fatal(err)
		}
		if st.Flags&ufHidden != 0 {
			t.Fatalf("force=%v: published file still has UF_HIDDEN (flags %#x)", force, st.Flags)
		}
	}
}
