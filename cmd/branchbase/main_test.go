package main

import (
	"testing"

	"github.com/branchbase/branchbase/internal/config"
)

func TestStatusDatabaseNameNilConfig(t *testing.T) {
	got := statusDatabaseName(nil, "feature-x")
	want := "myapp_dev_feature-x"
	if got != want {
		t.Fatalf("statusDatabaseName(nil, feature-x) = %q, want %q", got, want)
	}
}

func TestStatusDatabaseNameUsesConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	got := statusDatabaseName(&cfg, "feature-x")
	want := cfg.DatabaseNameForBranch("feature-x")
	if got != want {
		t.Fatalf("statusDatabaseName(&cfg, feature-x) = %q, want %q", got, want)
	}
}
