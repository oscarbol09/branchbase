package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Driver != "postgres" {
		t.Errorf("expected driver 'postgres', got %q", cfg.Driver)
	}
	if cfg.Proxy.ListenPort != 5432 {
		t.Errorf("expected proxy port 5432, got %d", cfg.Proxy.ListenPort)
	}
}

func TestDatabaseNameForBranch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Connection.BaseDatabase = "myapp_dev"
	cfg.Proxy.DefaultBranch = "main"

	tests := []struct {
		branch   string
		expected string
	}{
		{"main", "myapp_dev"},
		{"", "myapp_dev"},
		{"feature_billing", "myapp_dev_feature_billing"},
		{"hotfix_login", "myapp_dev_hotfix_login"},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			got := cfg.DatabaseNameForBranch(tt.branch)
			if got != tt.expected {
				t.Errorf("DatabaseNameForBranch(%q) = %q; want %q", tt.branch, got, tt.expected)
			}
		})
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "branchbase_config_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	cfg := DefaultConfig()
	cfg.Connection.BaseDatabase = "custom_dev_db"

	jsonPath := filepath.Join(tempDir, ".branchbase.json")
	if err := cfg.SaveJSON(jsonPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := LoadConfig(tempDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Connection.BaseDatabase != "custom_dev_db" {
		t.Errorf("expected 'custom_dev_db', got %q", loaded.Connection.BaseDatabase)
	}
}
