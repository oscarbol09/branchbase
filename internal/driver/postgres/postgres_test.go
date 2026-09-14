package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/branchbase/branchbase/internal/driver"
)

func TestFormatDBName(t *testing.T) {
	t.Parallel()
	d, err := New(Config{
		BaseDatabase: "myapp_dev",
	})
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	tests := []struct {
		branch   string
		expected string
	}{
		{"main", "myapp_dev"},
		{"master", "myapp_dev"},
		{"", "myapp_dev"},
		{"   ", "myapp_dev"},
		{"feature_billing", "myapp_dev_feature_billing"},
		{"hotfix_auth", "myapp_dev_hotfix_auth"},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			t.Parallel()
			got := d.formatDBName(tt.branch)
			if got != tt.expected {
				t.Errorf("formatDBName(%q) = %q; want %q", tt.branch, got, tt.expected)
			}
		})
	}
}

func TestDriverName(t *testing.T) {
	t.Parallel()
	d, _ := New(Config{BaseDatabase: "myapp_dev"})
	if d.Name() != "postgres" {
		t.Errorf("expected driver name 'postgres', got %q", d.Name())
	}
}

func TestDeleteBranchProtectedBaseDatabase(t *testing.T) {
	t.Parallel()
	d, err := New(Config{BaseDatabase: "myapp_dev"})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ctx := context.Background()

	protectedBranches := []string{"main", "master", "", "   "}
	for _, branch := range protectedBranches {
		err := d.DeleteBranch(ctx, branch)
		if err == nil {
			t.Errorf("expected error deleting protected branch %q, got nil", branch)
		}
		if !strings.Contains(err.Error(), "cannot delete protected base database") {
			t.Errorf("expected protected database error for branch %q, got: %v", branch, err)
		}
	}
}

func TestPingUninitialized(t *testing.T) {
	t.Parallel()
	d, err := New(Config{BaseDatabase: "myapp_dev"})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = d.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error on uninitialized Ping, got nil")
	}
	if !strings.Contains(err.Error(), "postgres connection not initialized") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestNewDefaultSSLMode(t *testing.T) {
	t.Parallel()
	d, err := New(Config{BaseDatabase: "test_db", SSLMode: ""})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if d.cfg.SSLMode != "disable" {
		t.Errorf("expected default SSLMode 'disable', got %q", d.cfg.SSLMode)
	}
}

func TestPostgresDriverRegistered(t *testing.T) {
	t.Parallel()
	drv, err := driver.GetDriver("postgres", map[string]interface{}{
		"base_database": "custom_db",
		"port":          5432,
	})
	if err != nil {
		t.Fatalf("GetDriver failed: %v", err)
	}
	if drv.Name() != "postgres" {
		t.Errorf("expected 'postgres', got %q", drv.Name())
	}
}
