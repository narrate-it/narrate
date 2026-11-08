//go:build darwin

package cli

import "syscall"

// ufHidden is UF_HIDDEN from <sys/stat.h>.
const ufHidden = 0x8000

// clearHidden removes the Finder "hidden" flag from a published file.
// Outputs are staged as dot-prefixed temp files; on iCloud-synced folders
// such as ~/Desktop the file provider marks dotfiles UF_HIDDEN, and the flag
// survives the rename to the visible destination name.
func clearHidden(path string) error {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return err
	}
	if st.Flags&ufHidden == 0 {
		return nil
	}
	return syscall.Chflags(path, int(st.Flags&^ufHidden))
}
