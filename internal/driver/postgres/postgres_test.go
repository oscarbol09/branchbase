package postgres

import (
	"testing"
)

func TestFormatDBName(t *testing.T) {
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
		{"feature_billing", "myapp_dev_feature_billing"},
		{"hotfix_auth", "myapp_dev_hotfix_auth"},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			got := d.formatDBName(tt.branch)
			if got != tt.expected {
				t.Errorf("formatDBName(%q) = %q; want %q", tt.branch, got, tt.expected)
			}
		})
	}
}

func TestDriverName(t *testing.T) {
	d, _ := New(Config{BaseDatabase: "myapp_dev"})
	if d.Name() != "postgres" {
		t.Errorf("expected driver name 'postgres', got %q", d.Name())
	}
}
