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

	tests := []struct {
		name     string
		defaultB string
		branch   string
		expected string
	}{
		{name: "main default", defaultB: "main", branch: "main", expected: "myapp_dev"},
		{name: "empty branch", defaultB: "main", branch: "", expected: "myapp_dev"},
		{name: "feature", defaultB: "main", branch: "feature_billing", expected: "myapp_dev_feature_billing"},
		{name: "hotfix", defaultB: "main", branch: "hotfix_login", expected: "myapp_dev_hotfix_login"},
		{name: "slash default matches sanitized", defaultB: "release/v1", branch: "release_v1", expected: "myapp_dev"},
		{name: "slash default other branch", defaultB: "release/v1", branch: "feature_x", expected: "myapp_dev_feature_x"},
		{name: "hyphen default matches sanitized", defaultB: "master-staging", branch: "master_staging", expected: "myapp_dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := DefaultConfig()
			cfg.Connection.BaseDatabase = "myapp_dev"
			cfg.Proxy.DefaultBranch = tt.defaultB
			got := cfg.DatabaseNameForBranch(tt.branch)
			if got != tt.expected {
				t.Errorf("DatabaseNameForBranch(%q) with DefaultBranch %q = %q; want %q", tt.branch, tt.defaultB, got, tt.expected)
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

func TestLoadConfigEnvExpansion(t *testing.T) {
	t.Setenv("BB_TEST_HOST", "db.example.internal")
	t.Setenv("BB_TEST_USER", "vault_user")
	t.Setenv("BB_TEST_PASS", "super-secret-pw")

	tempDir := t.TempDir()
	yamlPath := filepath.Join(tempDir, ".branchbase.yaml")
	content := []byte(`
connection:
  host: ${BB_TEST_HOST}
  user: ${BB_TEST_USER}
  password: ${BB_TEST_PASS}
  base_database: prod_dev
`)
	if err := os.WriteFile(yamlPath, content, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	loaded, err := LoadConfig(tempDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Connection.Host != "db.example.internal" {
		t.Errorf("expected host 'db.example.internal', got %q", loaded.Connection.Host)
	}
	if loaded.Connection.User != "vault_user" {
		t.Errorf("expected user 'vault_user', got %q", loaded.Connection.User)
	}
	if loaded.Connection.Password != "super-secret-pw" {
		t.Errorf("expected password 'super-secret-pw', got %q", loaded.Connection.Password)
	}
}

func TestLoadConfigYMLExtension(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	ymlPath := filepath.Join(tempDir, ".branchbase.yml")
	content := []byte(`
driver: sqlite
connection:
  base_database: yml_app_dev
`)
	if err := os.WriteFile(ymlPath, content, 0644); err != nil {
		t.Fatalf("failed to write .branchbase.yml: %v", err)
	}

	loaded, err := LoadConfig(tempDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Driver != "sqlite" {
		t.Errorf("expected driver 'sqlite', got %q", loaded.Driver)
	}
	if loaded.Connection.BaseDatabase != "yml_app_dev" {
		t.Errorf("expected base_database 'yml_app_dev', got %q", loaded.Connection.BaseDatabase)
	}
}
