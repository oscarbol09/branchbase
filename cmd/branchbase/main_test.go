package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/branchbase/branchbase/internal/config"
)

func TestStatusDatabaseNameNilConfig(t *testing.T) {
	t.Parallel()
	got := statusDatabaseName(nil, "feature-x")
	want := "myapp_dev_feature-x"
	if got != want {
		t.Fatalf("statusDatabaseName(nil, feature-x) = %q, want %q", got, want)
	}
}

func TestStatusDatabaseNameUsesConfig(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	got := statusDatabaseName(&cfg, "feature-x")
	want := cfg.DatabaseNameForBranch("feature-x")
	if got != want {
		t.Fatalf("statusDatabaseName(&cfg, feature-x) = %q, want %q", got, want)
	}
}

func TestStatusOutputJSONMarshal(t *testing.T) {
	t.Parallel()
	info := statusOutput{
		Branch:         "main",
		Sanitized:      "main",
		Database:       "myapp_dev_main",
		Driver:         "postgres",
		ProxyPort:      5432,
		Backend:        "127.0.0.1:5433",
		HooksInstalled: true,
	}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"branch", "sanitized", "database", "driver", "proxy_port", "backend", "hooks_installed"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q in JSON: %s", key, raw)
		}
	}
}

func TestBuildStatusMissingConfigDefaults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Not a git repo and no config — should still return graceful defaults, not panic.
	info, _ := buildStatus(dir)
	if info.Branch != "unknown" {
		t.Fatalf("branch = %q, want unknown", info.Branch)
	}
	if info.Driver != "postgres" {
		t.Fatalf("driver = %q, want postgres", info.Driver)
	}
	if info.Database == "" {
		t.Fatal("database should have a fallback value")
	}
	raw, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("invalid JSON: %s", raw)
	}
}

func TestBuildStatusWithConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Connection.BaseDatabase = "myapp_dev"
	cfg.Proxy.ListenPort = 5433
	cfg.Connection.Host = "127.0.0.1"
	cfg.Connection.Port = 5432
	if err := cfg.SaveJSON(filepath.Join(dir, ".branchbase.json")); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}
	// init a git repo on main so ResolveCurrentBranch works
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	// Without a real git repo ResolveCurrentBranch may fail — that's ok; focus on config fields.
	info, _ := buildStatus(dir)
	if info.Driver != "postgres" {
		t.Fatalf("driver = %q", info.Driver)
	}
	if info.ProxyPort != 5433 {
		t.Fatalf("proxy_port = %d, want 5433", info.ProxyPort)
	}
	if info.Backend != "127.0.0.1:5432" {
		t.Fatalf("backend = %q, want 127.0.0.1:5432", info.Backend)
	}
}
