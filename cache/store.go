package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store is a simple filesystem-backed cache store.
type Store struct {
	Dir string
}

// NewStore creates a cache store rooted at dir.
func NewStore(dir string) *Store {
	return &Store{Dir: dir}
}

// Path returns the on-disk path for a cache entry.
func (s *Store) Path(key Key) string {
	return filepath.Join(s.Dir, key.Backend, key.String()+".json")
}

// Get returns a cached artifact payload when present.
func (s *Store) Get(key Key) ([]byte, bool, error) {
	data, err := os.ReadFile(s.Path(key))
	if err == nil {
		return data, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

// Put stores a cache payload atomically.
func (s *Store) Put(key Key, payload []byte) error {
	path := s.Path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "lfx-cache-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// PutJSON marshals a value and writes it into the cache.
func (s *Store) PutJSON(key Key, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshalling cache payload: %w", err)
	}
	return s.Put(key, payload)
}
