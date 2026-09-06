package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"main", "main"},
		{"master", "master"},
		{"feature/billing-v2", "feature_billing_v2"},
		{"fix/issue#123-crash", "fix_issue_123_crash"},
		{"user/john.doe/experimental_test", "user_john_doe_experimental_test"},
		{"--leading-and-trailing--", "leading_and_trailing"},
		{"", "default"},
		{"   ", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeBranchName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeBranchName(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResolveCurrentBranch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "branchbase_git_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	gitDir := filepath.Join(tempDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	// Write mock HEAD pointing to refs/heads/feature/auth-provider
	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/feature/auth-provider\n"), 0644); err != nil {
		t.Fatalf("failed to write HEAD: %v", err)
	}

	branch, err := ResolveCurrentBranch(tempDir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if branch != "feature/auth-provider" {
		t.Errorf("expected branch 'feature/auth-provider', got %q", branch)
	}
}
