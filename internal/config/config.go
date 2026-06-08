package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const appDirName = "Ignore"

type Config struct {
	Enabled          bool   `json:"enabled"`
	StartWithWindows bool   `json:"startWithWindows"`
	WorkerCount      int    `json:"workerCount"`
	BufferSize       int    `json:"bufferSize"`
	GlobalIgnorePath string `json:"globalIgnorePath"`
	LogDir           string `json:"logDir"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func NewStore() (*Store, error) {
	dir, err := DataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	cfg := Config{
		Enabled:          true,
		StartWithWindows: true,
		WorkerCount:      0,
		BufferSize:       1024 * 1024,
		GlobalIgnorePath: filepath.Join(home, ".ignore"),
		LogDir:           filepath.Join(dir, "logs"),
	}
	store := &Store{path: filepath.Join(dir, "config.json"), cfg: cfg}
	if err := store.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := store.Save(); err != nil {
			return nil, err
		}
	}
	if err := ensureGlobalIgnore(store.cfg.GlobalIgnorePath); err != nil {
		return nil, err
	}
	return store, nil
}

func DataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDirName), nil
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	b = []byte(strings.TrimPrefix(string(b), "\ufeff"))
	cfg := s.cfg
	if err := json.Unmarshal(b, &cfg); err != nil {
		return err
	}
	s.cfg = cfg
	return nil
}

func (s *Store) Save() error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *Store) Update(fn func(*Config)) error {
	s.mu.Lock()
	fn(&s.cfg)
	s.mu.Unlock()
	return s.Save()
}

func ensureGlobalIgnore(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, []byte(DefaultIgnoreContent()), 0o644)
}

func DefaultIgnoreContent() string {
	return `# Ignore rules
# You can paste .gitignore content directly into this file.

node_modules/
vendor/
vendors/
dist
build
.next
.git

.env
.env.local

*.log
*.tmp
*.cache
*.bak
`
}
