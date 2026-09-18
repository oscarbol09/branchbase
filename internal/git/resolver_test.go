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

func TestResolveLocalBranchesLoose(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	headsDir := filepath.Join(tempDir, ".git", "refs", "heads", "feature")
	if err := os.MkdirAll(headsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mainFile := filepath.Join(tempDir, ".git", "refs", "heads", "main")
	if err := os.WriteFile(mainFile, []byte("commit-hash-1\n"), 0644); err != nil {
		t.Fatalf("write main: %v", err)
	}

	featFile := filepath.Join(headsDir, "auth")
	if err := os.WriteFile(featFile, []byte("commit-hash-2\n"), 0644); err != nil {
		t.Fatalf("write feat: %v", err)
	}

	branches, err := ResolveLocalBranches(tempDir)
	if err != nil {
		t.Fatalf("ResolveLocalBranches failed: %v", err)
	}

	branchMap := make(map[string]bool)
	for _, b := range branches {
		branchMap[b] = true
	}

	if !branchMap["main"] {
		t.Errorf("expected 'main' in branches, got %v", branches)
	}
	if !branchMap["feature/auth"] {
		t.Errorf("expected 'feature/auth' in branches, got %v", branches)
	}
}

func TestResolveLocalBranchesPacked(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	packedRefs := `# pack-refs with: peeled-tags
e02b7e1975e5330335e386992adbe813636f0de2 refs/heads/staging
b12f45c21975e5330335e386992adbe813636f0de refs/heads/feature/payments
^e02b7e1975e5330335e386992adbe813636f0de2
`
	if err := os.WriteFile(filepath.Join(gitDir, "packed-refs"), []byte(packedRefs), 0644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	branches, err := ResolveLocalBranches(tempDir)
	if err != nil {
		t.Fatalf("ResolveLocalBranches failed: %v", err)
	}

	branchMap := make(map[string]bool)
	for _, b := range branches {
		branchMap[b] = true
	}

	if !branchMap["staging"] {
		t.Errorf("expected 'staging' in branches, got %v", branches)
	}
	if !branchMap["feature/payments"] {
		t.Errorf("expected 'feature/payments' in branches, got %v", branches)
	}
}

func TestResolveLocalBranchesNotAGitRepo(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	_, err := ResolveLocalBranches(tempDir)
	if err == nil {
		t.Fatal("expected error when resolving branches in non-git directory, got nil")
	}
}

func TestResolveLocalBranchesFromFSWorktreeCommondir(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	// Main repo .git holds refs/heads and packed-refs.
	mainGit := filepath.Join(tempDir, "main_repo", ".git")
	headsDir := filepath.Join(mainGit, "refs", "heads", "feature")
	if err := os.MkdirAll(headsDir, 0755); err != nil {
		t.Fatalf("mkdir heads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainGit, "refs", "heads", "main"), []byte("hash-main\n"), 0644); err != nil {
		t.Fatalf("write main: %v", err)
	}
	if err := os.WriteFile(filepath.Join(headsDir, "auth"), []byte("hash-auth\n"), 0644); err != nil {
		t.Fatalf("write feat: %v", err)
	}
	packed := "e02b7e1975e5330335e386992adbe813636f0de2 refs/heads/staging\n"
	if err := os.WriteFile(filepath.Join(mainGit, "packed-refs"), []byte(packed), 0644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	// Linked worktree git dir: HEAD lives here; commondir points at main .git.
	wtGit := filepath.Join(mainGit, "worktrees", "wt1")
	if err := os.MkdirAll(wtGit, 0755); err != nil {
		t.Fatalf("mkdir worktree git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtGit, "HEAD"), []byte("ref: refs/heads/feature/auth\n"), 0644); err != nil {
		t.Fatalf("write worktree HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtGit, "commondir"), []byte("../..\n"), 0644); err != nil {
		t.Fatalf("write commondir: %v", err)
	}

	wtDir := filepath.Join(tempDir, "wt_copy")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatalf("mkdir wt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtDir, ".git"), []byte("gitdir: "+wtGit+"\n"), 0644); err != nil {
		t.Fatalf("write .git file: %v", err)
	}

	branches, err := resolveLocalBranchesFromFS(wtDir)
	if err != nil {
		t.Fatalf("resolveLocalBranchesFromFS: %v", err)
	}
	got := make(map[string]bool, len(branches))
	for _, b := range branches {
		got[b] = true
	}
	for _, want := range []string{"main", "feature/auth", "staging"} {
		if !got[want] {
			t.Errorf("expected %q in branches, got %v", want, branches)
		}
	}
}
