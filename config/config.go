package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Manager struct {
	mu   sync.RWMutex
	path string
	cfg  *AppConfig
}

func NewManager() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not find home directory: %w", err)
	}

	dir := filepath.Join(home, ".dockcode")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("could not create config directory: %w", err)
	}

	path := filepath.Join(dir, "config.json")
	return &Manager{
		path: path,
		cfg:  DefaultConfig(),
	}, nil
}
func (m *Manager) ConfigExists() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, err := os.Stat(m.path)
	return err == nil
}
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config JSON: %w", err)
	}

	m.cfg = &cfg
	return nil
}
func (m *Manager) Save() error {
	m.mu.RLock()
	cfg := *m.cfg
	m.mu.RUnlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	tmpFile := m.path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	if err := os.Rename(tmpFile, m.path); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename temp config file: %w", err)
	}

	return nil
}
func (m *Manager) Get() AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.cfg
}
func (m *Manager) Update(fn func(*AppConfig)) error {
	m.mu.Lock()
	fn(m.cfg)
	m.mu.Unlock()

	return m.Save()
}
