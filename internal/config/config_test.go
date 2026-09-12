package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.Driver != "postgres" {
		t.Errorf("expected driver 'postgres', got %q", cfg.Driver)
	}
	if cfg.Proxy.ListenPort != 5432 {
		t.Errorf("expected proxy port 5432, got %d", cfg.Proxy.ListenPort)
	}
}

func TestDatabaseNameForBranch(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			got := cfg.DatabaseNameForBranch(tt.branch)
			if got != tt.expected {
				t.Errorf("DatabaseNameForBranch(%q) = %q; want %q", tt.branch, got, tt.expected)
			}
		})
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

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

func TestLoadConfigNotFound(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	_, err := LoadConfig(tempDir)
	if err == nil {
		t.Fatal("expected error when loading config from empty directory, got nil")
	}
}

func TestLoadConfigCorruptJSON(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	jsonPath := filepath.Join(tempDir, ".branchbase.json")
	if err := os.WriteFile(jsonPath, []byte(`{invalid-json`), 0644); err != nil {
		t.Fatalf("failed to write corrupt config: %v", err)
	}

	_, err := LoadConfig(tempDir)
	if err == nil {
		t.Fatal("expected unmarshal error when loading corrupt json, got nil")
	}
}
