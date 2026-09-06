package git

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrNotAGitRepo     = errors.New("not a git repository (missing .git)")
	ErrDetachedHEAD    = errors.New("git repository is in detached HEAD state")
	ErrUnreadableHEAD  = errors.New("cannot read .git/HEAD")
	invalidCharsRegex  = regexp.MustCompile(`[^a-zA-Z0-9_]`)
	multipleUnderscore = regexp.MustCompile(`_+`)
)

// ResolveCurrentBranch inspects .git/HEAD in the specified repository path
// and returns the active branch name.
func ResolveCurrentBranch(repoPath string) (string, error) {
	gitDir := filepath.Join(repoPath, ".git")
	headPath := filepath.Join(gitDir, "HEAD")

	// If .git is a file (common in git worktrees or submodules)
	fileInfo, err := os.Stat(gitDir)
	if err != nil {
		return "", ErrNotAGitRepo
	}

	if !fileInfo.IsDir() {
		// Read gitdir pointer for worktrees
		content, err := os.ReadFile(gitDir)
		if err != nil {
			return "", ErrNotAGitRepo
		}
		text := strings.TrimSpace(string(content))
		if strings.HasPrefix(text, "gitdir: ") {
			targetDir := strings.TrimPrefix(text, "gitdir: ")
			if !filepath.IsAbs(targetDir) {
				targetDir = filepath.Join(repoPath, targetDir)
			}
			headPath = filepath.Join(targetDir, "HEAD")
		}
	}

	headBytes, err := os.ReadFile(headPath)
	if err != nil {
		return "", ErrUnreadableHEAD
	}

	headContent := strings.TrimSpace(string(headBytes))

	// Normal branch format: "ref: refs/heads/<branch_name>"
	const refPrefix = "ref: refs/heads/"
	if strings.HasPrefix(headContent, refPrefix) {
		branch := strings.TrimPrefix(headContent, refPrefix)
		if branch == "" {
			return "", ErrUnreadableHEAD
		}
		return branch, nil
	}

	// If it contains a raw commit hash (40 chars), we are in detached HEAD
	if len(headContent) >= 40 && !strings.Contains(headContent, " ") {
		return "", ErrDetachedHEAD
	}

	return "", ErrUnreadableHEAD
}

// SanitizeBranchName converts a Git branch name (which can contain '/', '-', '.')
// into a safe, valid SQL identifier name for databases.
//
// Examples:
//
//	"feature/payment-v2" -> "feature_payment_v2"
//	"hotfix/login.bug"   -> "hotfix_login_bug"
//	"main"               -> "main"
func SanitizeBranchName(branch string) string {
	branch = strings.TrimSpace(branch)
	sanitized := invalidCharsRegex.ReplaceAllString(branch, "_")
	sanitized = multipleUnderscore.ReplaceAllString(sanitized, "_")
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" {
		return "default"
	}
	return strings.ToLower(sanitized)
}
