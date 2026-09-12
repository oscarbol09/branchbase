package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeBranchName(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			result := SanitizeBranchName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeBranchName(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func FuzzSanitizeBranchName(f *testing.F) {
	seeds := []string{"main", "feature/auth", "v1.0.0-rc1", "weird#branch!name", "", "   ", "---"}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		sanitized := SanitizeBranchName(input)
		if sanitized == "" {
			t.Errorf("SanitizeBranchName(%q) returned empty string", input)
		}
	})
}

func TestResolveCurrentBranch(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

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

func TestResolveCurrentBranchNotAGitRepo(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	_, err := ResolveCurrentBranch(tempDir)
	if err != ErrNotAGitRepo {
		t.Fatalf("expected ErrNotAGitRepo, got: %v", err)
	}
}

func TestResolveCurrentBranchDetachedHEAD(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	gitDir := filepath.Join(tempDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git: %v", err)
	}

	commitHash := "e02b7e1975e5330335e386992adbe813636f0de2\n"
	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte(commitHash), 0644); err != nil {
		t.Fatalf("failed to write HEAD: %v", err)
	}

	_, err := ResolveCurrentBranch(tempDir)
	if err != ErrDetachedHEAD {
		t.Fatalf("expected ErrDetachedHEAD, got: %v", err)
	}
}

func TestResolveCurrentBranchUnreadableHEAD(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	gitDir := filepath.Join(tempDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git: %v", err)
	}

	// Empty HEAD or invalid ref
	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/\n"), 0644); err != nil {
		t.Fatalf("failed to write HEAD: %v", err)
	}

	_, err := ResolveCurrentBranch(tempDir)
	if err != ErrUnreadableHEAD {
		t.Fatalf("expected ErrUnreadableHEAD, got: %v", err)
	}
}

func TestResolveCurrentBranchWorktree(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	// Real git dir elsewhere
	realGitDir := filepath.Join(tempDir, "main_repo", ".git", "worktrees", "wt1")
	if err := os.MkdirAll(realGitDir, 0755); err != nil {
		t.Fatalf("failed to create worktree git dir: %v", err)
	}

	headPath := filepath.Join(realGitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/feature/worktree-branch\n"), 0644); err != nil {
		t.Fatalf("failed to write worktree HEAD: %v", err)
	}

	// Worktree working copy has a .git file
	wtDir := filepath.Join(tempDir, "wt_copy")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("failed to create wt dir: %v", err)
	}

	gitFile := filepath.Join(wtDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+realGitDir+"\n"), 0644); err != nil {
		t.Fatalf("failed to write .git file: %v", err)
	}

	branch, err := ResolveCurrentBranch(wtDir)
	if err != nil {
		t.Fatalf("expected no error for worktree, got: %v", err)
	}
	if branch != "feature/worktree-branch" {
		t.Errorf("expected 'feature/worktree-branch', got %q", branch)
	}
}
