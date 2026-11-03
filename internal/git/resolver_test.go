package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestResolveCurrentBranchRebase(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	// In detached HEAD during rebase, HEAD has a 40-char SHA
	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("0123456789abcdef0123456789abcdef01234567\n"), 0644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}

	// Git creates .git/rebase-merge/head-name with the target branch
	rebaseDir := filepath.Join(gitDir, "rebase-merge")
	if err := os.MkdirAll(rebaseDir, 0755); err != nil {
		t.Fatalf("mkdir rebase-merge: %v", err)
	}
	headNamePath := filepath.Join(rebaseDir, "head-name")
	if err := os.WriteFile(headNamePath, []byte("refs/heads/feature/checkout-v2\n"), 0644); err != nil {
		t.Fatalf("write head-name: %v", err)
	}

	branch, err := ResolveCurrentBranch(tempDir)
	if err != nil {
		t.Fatalf("expected active branch during rebase, got error: %v", err)
	}
	if branch != "feature/checkout-v2" {
		t.Errorf("expected 'feature/checkout-v2', got %q", branch)
	}
}

func TestValidateBranchNameUnique(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	gitDir := filepath.Join(tempDir, ".git")
	headsDir := filepath.Join(gitDir, "refs", "heads")
	if err := os.MkdirAll(headsDir, 0755); err != nil {
		t.Fatalf("mkdir refs/heads: %v", err)
	}

	// Create branch files: "main" and "feat/order-pay"
	featDir := filepath.Join(headsDir, "feat")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		t.Fatalf("mkdir refs/heads/feat: %v", err)
	}
	if err := os.WriteFile(filepath.Join(headsDir, "main"), []byte("sha1\n"), 0644); err != nil {
		t.Fatalf("write main: %v", err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "order-pay"), []byte("sha2\n"), 0644); err != nil {
		t.Fatalf("write order-pay: %v", err)
	}

	// Case 1: Validating the existing branch itself should succeed
	if err := ValidateBranchNameUnique(tempDir, "feat/order-pay"); err != nil {
		t.Errorf("expected feat/order-pay to be valid against itself, got: %v", err)
	}

	// Case 2: A different branch name that sanitizes to the SAME identifier must FAIL
	// "feat/order-pay" -> "feat_order_pay"
	// "feat_order_pay" -> "feat_order_pay" (collision!)
	err := ValidateBranchNameUnique(tempDir, "feat_order_pay")
	if err == nil {
		t.Fatal("expected collision error for 'feat_order_pay', got nil")
	}
	if !strings.Contains(err.Error(), "map to the same database identifier") {
		t.Errorf("expected error message to explain collision, got: %v", err)
	}

	// Case 3: A genuinely unique branch should succeed
	if err := ValidateBranchNameUnique(tempDir, "feature-new"); err != nil {
		t.Errorf("expected unique branch to pass validation, got: %v", err)
	}
}

func TestResolveLocalBranchesContext_Cancellation(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	// Calling with canceled context should abort without hanging
	_, _ = ResolveLocalBranchesContext(ctx, tempDir)
}

func TestResolveMergedBranchesRemoteTrackingFallback(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-b", "feature/merged")
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	git("branch", "feature/unmerged")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	git("switch", "feature/unmerged")
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "unmerged")
	git("switch", "feature/merged")

	got, err := ResolveMergedBranches(repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "feature/merged" {
		t.Fatalf("merged branches = %v, want only feature/merged", got)
	}
}

func TestResolveMergedBranchesPrefersLocalDefault(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	git("branch", "feature/merged")
	git("switch", "-c", "feature/unmerged")
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "remote-ahead")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	git("switch", "feature/merged")

	got, err := ResolveMergedBranches(repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "feature/merged" {
		t.Fatalf("merged branches = %v, want only feature/merged", got)
	}
}

func TestResolveMergedBranchesMissingDefault(t *testing.T) {
	repo := t.TempDir()
	cmd := exec.Command("git", "-C", repo, "init", "-b", "feature/only")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if _, err := ResolveMergedBranches(repo, "main"); err == nil {
		t.Fatal("expected error when neither main nor origin/main exists")
	}
}

func TestResolveMergedBranchesContext_Cancellation(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = ResolveMergedBranchesContext(ctx, tempDir, "main")
}
