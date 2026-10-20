package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type DaemonConfig struct {
	Port        int             `json:"port"`
	Whitelist   map[string]bool `json:"whitelist"`
	DebounceSec int             `json:"debounceSec"`
	filePath    string
	mu          sync.RWMutex
}

func LoadConfig(path string) (*DaemonConfig, error) {
	cfg := &DaemonConfig{
		Port:        18080,
		Whitelist:   make(map[string]bool),
		DebounceSec: 3,
		filePath:    path,
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create config dir: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Write default config
			if saveErr := cfg.saveLocked(); saveErr != nil {
				return nil, saveErr
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config json: %w", err)
	}
	if cfg.Whitelist == nil {
		cfg.Whitelist = make(map[string]bool)
	}
	if cfg.Port == 0 {
		cfg.Port = 18080
	}
	if cfg.DebounceSec == 0 {
		cfg.DebounceSec = 3
	}
	cfg.filePath = path

	return cfg, nil
}

func (c *DaemonConfig) IsEnabled(username string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Whitelist[username]
}

func (c *DaemonConfig) SetEnabled(username string, enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Whitelist[username] = enabled
	return c.saveLocked()
}

func (c *DaemonConfig) GetWhitelist() map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	res := make(map[string]bool, len(c.Whitelist))
	for k, v := range c.Whitelist {
		res[k] = v
	}
	return res
}

func (c *DaemonConfig) saveLocked() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	tmpFile := c.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write tmp config: %w", err)
	}

	return os.Rename(tmpFile, c.filePath)
}
