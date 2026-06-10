package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileStore is a PluginStore backed by a single JSON file. It keeps remote plugin installs and
// per-plugin configuration across daemon restarts without requiring the logbook database.
type FileStore struct {
	mu   sync.Mutex
	path string
}

// NewFileStore creates a FileStore persisting to path (parent directories are created on save).
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) load() (map[string]*PluginRecord, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]*PluginRecord{}, nil
		}
		return nil, fmt.Errorf("pluginstore: read: %w", err)
	}
	var records map[string]*PluginRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("pluginstore: parse: %w", err)
	}
	if records == nil {
		records = map[string]*PluginRecord{}
	}
	return records, nil
}

func (s *FileStore) save(records map[string]*PluginRecord) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("pluginstore: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("pluginstore: mkdir: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("pluginstore: write: %w", err)
	}
	return os.Rename(tmp, s.path)
}

// List returns all persisted plugin records.
func (s *FileStore) List(_ context.Context) ([]*PluginRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]*PluginRecord, 0, len(records))
	for _, r := range records {
		out = append(out, r)
	}
	return out, nil
}

// Save upserts one plugin record.
func (s *FileStore) Save(_ context.Context, r *PluginRecord) error {
	if r == nil || r.ID == "" {
		return errors.New("pluginstore: record needs an id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.load()
	if err != nil {
		return err
	}
	records[r.ID] = r
	return s.save(records)
}

// Delete removes one plugin record (no error when absent).
func (s *FileStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.load()
	if err != nil {
		return err
	}
	delete(records, id)
	return s.save(records)
}
