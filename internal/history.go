package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// HistoryEntry records a completed download.
type HistoryEntry struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Format       string    `json:"format"`
	Quality      string    `json:"quality"`
	OutputPath   string    `json:"outputPath"`
	FileSize     int64     `json:"fileSize"`
	ThumbnailURL string    `json:"thumbnailUrl"`
	CompletedAt  time.Time `json:"completedAt"`
}

// HistoryStore manages persistent download history.
type HistoryStore struct {
	mu      sync.Mutex
	path    string
	entries []HistoryEntry
}

func NewHistoryStore(configDir string) (*HistoryStore, error) {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(configDir, "history.json")
	s := &HistoryStore{path: path}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *HistoryStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.entries)
}

func (s *HistoryStore) save() error {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Append adds a new entry and persists to disk.
func (s *HistoryStore) Append(e HistoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	return s.save()
}

// GetAll returns history sorted by completedAt descending (max 200).
func (s *HistoryStore) GetAll() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]HistoryEntry, len(s.entries))
	copy(result, s.entries)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CompletedAt.After(result[j].CompletedAt)
	})
	if len(result) > 200 {
		result = result[:200]
	}
	return result
}

// Clear removes all history entries.
func (s *HistoryStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
	return s.save()
}
