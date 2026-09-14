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
	cfg.Connection.Port = 15432
	cfg.Connection.User = "json_user"
	cfg.Connection.BaseDatabase = "custom_dev_db"

	jsonPath := filepath.Join(tempDir, ".branchbase.json")
	if err := cfg.SaveJSON(jsonPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := LoadConfig(tempDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Connection.Port != 15432 {
		t.Errorf("expected port 15432, got %d", loaded.Connection.Port)
	}
	if loaded.Connection.User != "json_user" {
		t.Errorf("expected user 'json_user', got %q", loaded.Connection.User)
	}
	if loaded.Connection.BaseDatabase != "custom_dev_db" {
		t.Errorf("expected 'custom_dev_db', got %q", loaded.Connection.BaseDatabase)
	}
}

func TestLoadConfigYAML(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	yamlPath := filepath.Join(tempDir, ".branchbase.yaml")
	content := []byte(`
connection:
  port: 6543
  user: yaml_user
  base_database: yaml_dev
`)
	if err := os.WriteFile(yamlPath, content, 0644); err != nil {
		t.Fatalf("failed to write yaml config: %v", err)
	}

	loaded, err := LoadConfig(tempDir)
	if err != nil {
		t.Fatalf("failed to load yaml config: %v", err)
	}

	if loaded.Connection.Port != 6543 {
		t.Errorf("expected port 6543, got %d", loaded.Connection.Port)
	}
	if loaded.Connection.User != "yaml_user" {
		t.Errorf("expected user 'yaml_user', got %q", loaded.Connection.User)
	}
	if loaded.Connection.BaseDatabase != "yaml_dev" {
		t.Errorf("expected base_database 'yaml_dev', got %q", loaded.Connection.BaseDatabase)
	}
	// Unspecified fields should keep DefaultConfig values (same merge style as JSON).
	if loaded.Connection.Host != "127.0.0.1" {
		t.Errorf("expected default host '127.0.0.1', got %q", loaded.Connection.Host)
	}
	if loaded.Driver != "postgres" {
		t.Errorf("expected default driver 'postgres', got %q", loaded.Driver)
	}
}

func TestLoadConfigCorruptYAML(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	yamlPath := filepath.Join(tempDir, ".branchbase.yaml")
	if err := os.WriteFile(yamlPath, []byte("connection: [\n  invalid: yaml"), 0644); err != nil {
		t.Fatalf("failed to write corrupt yaml: %v", err)
	}

	_, err := LoadConfig(tempDir)
	if err == nil {
		t.Fatal("expected unmarshal error when loading corrupt yaml, got nil")
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
