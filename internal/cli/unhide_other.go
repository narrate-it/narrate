//go:build !darwin

package cli

// clearHidden is a no-op outside macOS, which has no hidden file flag.
func clearHidden(string) error { return nil }
