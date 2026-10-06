package mongodb

import (
	"testing"

	"github.com/branchbase/branchbase/internal/driver"
)

func TestMongoDBDriverRegistration(t *testing.T) {
	t.Parallel()

	// Test missing URI
	_, err := driver.GetDriver("mongodb", map[string]interface{}{
		"base_database": "testdb",
	})
	if err == nil {
		t.Error("expected error when uri is missing, got nil")
	}

	// Test missing BaseDatabase
	_, err = driver.GetDriver("mongodb", map[string]interface{}{
		"uri": "mongodb://localhost:27017",
	})
	if err == nil {
		t.Error("expected error when base_database is missing, got nil")
	}
}

func TestFormatDBName(t *testing.T) {
	t.Parallel()

	d := &MongoDBDriver{
		cfg: Config{
			BaseDatabase:  "app_dev",
			DefaultBranch: "main",
		},
	}

	tests := []struct {
		name     string
		branch   string
		expected string
	}{
		{
			name:     "default branch matches base database",
			branch:   "main",
			expected: "app_dev",
		},
		{
			name:     "feature branch with slash",
			branch:   "feature/auth",
			expected: "app_dev_feature_auth",
		},
		{
			name:     "hotfix branch with dashes and special characters",
			branch:   "hotfix-123.4",
			expected: "app_dev_hotfix_123_4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := d.formatDBName(tt.branch)
			if got != tt.expected {
				t.Errorf("formatDBName(%q) = %q, want %q", tt.branch, got, tt.expected)
			}
		})
	}
}

func TestMongoDBDriverName(t *testing.T) {
	t.Parallel()

	d := &MongoDBDriver{}
	if d.Name() != "mongodb" {
		t.Errorf("expected driver name 'mongodb', got %q", d.Name())
	}
}
