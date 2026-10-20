package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDaemonConfig_LoadAndSave(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "daemon_config.json")

	// 1. Initial load from non-existent file creates default config
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Port != 18080 {
		t.Errorf("expected default port 18080, got %d", cfg.Port)
	}
	if cfg.DebounceSec != 3 {
		t.Errorf("expected default DebounceSec 3, got %d", cfg.DebounceSec)
	}

	// 2. Set contact enabled
	contact := "wxid_7ujao93nx6wy22"
	if cfg.IsEnabled(contact) {
		t.Errorf("expected contact to be initially disabled")
	}

	err = cfg.SetEnabled(contact, true)
	if err != nil {
		t.Fatalf("SetEnabled failed: %v", err)
	}
	if !cfg.IsEnabled(contact) {
		t.Errorf("expected contact to be enabled after SetEnabled(true)")
	}

	// 3. Reload from disk to verify persistence
	cfg2, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("reload LoadConfig failed: %v", err)
	}
	if !cfg2.IsEnabled(contact) {
		t.Errorf("expected contact to remain enabled in reloaded config")
	}
}

func TestDaemonConfig_Concurrency(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "daemon_config.json")
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			user := "user_test"
			_ = cfg.SetEnabled(user, id%2 == 0)
			_ = cfg.IsEnabled(user)
		}(i)
	}
	wg.Wait()
}
