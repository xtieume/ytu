package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings holds all user-configurable options (persisted to disk).
type Settings struct {
	OutputDir        string `json:"outputDir"`
	MaxConcurrent    int    `json:"maxConcurrent"`
	DefaultFormat    string `json:"defaultFormat"`
	DefaultQuality   string `json:"defaultQuality"`
	EmbedThumbnail   bool   `json:"embedThumbnail"`
	Language         string `json:"language"`
	LogDir           string `json:"logDir"`
	LogRetentionDays int    `json:"logRetentionDays"`
}

// ServerInfo holds read-only runtime info returned alongside Settings.
type ServerInfo struct {
	Port      int  `json:"port"`
	HasFFmpeg bool `json:"hasFFmpeg"`
	HasYtdlp  bool `json:"hasYtdlp"`
	ActiveDL  int  `json:"activeDL"`
	MaxDL     int  `json:"maxDL"`
}

// SettingsResponse is what GET /api/settings returns.
type SettingsResponse struct {
	Settings
	ServerInfo
}

// SettingsStore manages persistent settings.
type SettingsStore struct {
	mu       sync.RWMutex
	path     string
	current  Settings
}

func NewSettingsStore(configDir string, defaults Settings) (*SettingsStore, error) {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, err
	}
	s := &SettingsStore{
		path:    filepath.Join(configDir, "settings.json"),
		current: defaults,
	}
	// Load saved settings, merging over defaults
	if data, err := os.ReadFile(s.path); err == nil {
		saved := defaults // start from defaults
		if json.Unmarshal(data, &saved) == nil {
			s.current = saved
		}
	}
	return s, nil
}

func (s *SettingsStore) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// SettingsPatch is used for partial updates from the API.
// Pointer fields let us distinguish "not sent" from explicit false/zero.
type SettingsPatch struct {
	OutputDir        string `json:"outputDir"`
	MaxConcurrent    int    `json:"maxConcurrent"`
	DefaultFormat    string `json:"defaultFormat"`
	DefaultQuality   string `json:"defaultQuality"`
	EmbedThumbnail   *bool  `json:"embedThumbnail"` // pointer: nil = not sent
	Language         string `json:"language"`
	LogDir           string `json:"logDir"`
	LogRetentionDays int    `json:"logRetentionDays"`
}

// Patch merges only the explicitly provided fields and persists.
func (s *SettingsStore) Patch(p SettingsPatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if p.OutputDir != "" {
		s.current.OutputDir = p.OutputDir
	}
	if p.MaxConcurrent > 0 {
		s.current.MaxConcurrent = p.MaxConcurrent
	}
	if p.DefaultFormat != "" {
		s.current.DefaultFormat = p.DefaultFormat
	}
	if p.DefaultQuality != "" {
		s.current.DefaultQuality = p.DefaultQuality
	}
	if p.EmbedThumbnail != nil {
		s.current.EmbedThumbnail = *p.EmbedThumbnail
	}
	if p.Language != "" {
		s.current.Language = p.Language
	}
	if p.LogDir != "" {
		s.current.LogDir = p.LogDir
	}
	if p.LogRetentionDays > 0 {
		s.current.LogRetentionDays = p.LogRetentionDays
	}

	return s.save()
}

func (s *SettingsStore) save() error {
	data, err := json.MarshalIndent(s.current, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
