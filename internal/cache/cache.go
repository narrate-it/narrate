// Package cache persists rewritten chunks and synthesized clips with
// settings-aware identity, restrictive permissions, and atomic writes.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store is a file-backed cache directory.
type Store struct{ Dir string }

// Open creates the directory with restrictive permissions if missing.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

// Key builds a hex cache key from ordered identity components.
// Credentials must never be passed here.
func Key(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Store) path(key, ext string) string {
	return filepath.Join(s.Dir, key+ext)
}

// GetJSON loads a cached JSON value; ok=false on any miss or corruption.
func (s *Store) GetJSON(key string, v any) (ok bool, err error) {
	data, err := os.ReadFile(s.path(key, ".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		// corrupt cache entry: treat as miss and remove
		_ = os.Remove(s.path(key, ".json"))
		return false, nil
	}
	return true, nil
}

// PutJSON atomically stores v, fsyncing before rename.
func (s *Store) PutJSON(key string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return s.writeAtomic(s.path(key, ".json"), data)
}

// PutFile atomically stores raw bytes (e.g. clips).
func (s *Store) PutFile(key, ext string, data []byte) error {
	return s.writeAtomic(s.path(key, ext), data)
}

// HasFile reports whether a cached file exists and is nonempty.
func (s *Store) HasFile(key, ext string) bool {
	fi, err := os.Stat(s.path(key, ext))
	return err == nil && fi.Size() > 0
}

// FilePath returns the absolute path of a cache file.
func (s *Store) FilePath(key, ext string) string {
	return s.path(key, ext)
}

// writeAtomic writes to a temp file in the same directory (0600), then renames.
func (s *Store) writeAtomic(dest string, data []byte) error {
	tmp, err := os.CreateTemp(s.Dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

// LockFile guards against two concurrent runs corrupting the same cache.
// It creates an exclusive lock file; a second run fails fast with guidance.
func (s *Store) LockFile() (release func(), err error) {
	p := filepath.Join(s.Dir, "lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("cache %s is locked by another run; if no run is active, delete %s", s.Dir, p)
		}
		return nil, err
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(p) }, nil
}

// Clear removes all cached content for the user.
func (s *Store) Clear() error {
	return os.RemoveAll(s.Dir)
}
