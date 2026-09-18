package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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

// ResolveLocalBranches returns all local branch names in the repository.
// It delegates to 'git for-each-ref' to account for both loose and packed refs,
// and falls back to inspecting .git/refs/heads and .git/packed-refs if git exec is unavailable.
func ResolveLocalBranches(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		var branches []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				branches = append(branches, trimmed)
			}
		}
		return branches, nil
	}

	return resolveLocalBranchesFromFS(repoPath)
}

// resolveGitCommonDir returns the directory that holds shared refs (refs/heads,
// packed-refs). Linked worktrees store those in the path named by commondir,
// not in .git/worktrees/<name>.
func resolveGitCommonDir(gitDir string) string {
	raw, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return gitDir
	}
	common := strings.TrimSpace(string(raw))
	if common == "" {
		return gitDir
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitDir, common)
	}
	return filepath.Clean(common)
}

func resolveLocalBranchesFromFS(repoPath string) ([]string, error) {
	gitDir := filepath.Join(repoPath, ".git")
	fi, err := os.Stat(gitDir)
	if err != nil {
		return nil, ErrNotAGitRepo
	}
	if !fi.IsDir() {
		content, err := os.ReadFile(gitDir)
		if err != nil {
			return nil, ErrNotAGitRepo
		}
		text := strings.TrimSpace(string(content))
		if strings.HasPrefix(text, "gitdir: ") {
			targetDir := strings.TrimPrefix(text, "gitdir: ")
			if !filepath.IsAbs(targetDir) {
				targetDir = filepath.Join(repoPath, targetDir)
			}
			gitDir = targetDir
		}
	}

	commonDir := resolveGitCommonDir(gitDir)

	seen := make(map[string]bool)
	var branches []string

	// 1. Walk .git/refs/heads (common dir for linked worktrees)
	headsDir := filepath.Join(commonDir, "refs", "heads")
	if headsInfo, err := os.Stat(headsDir); err == nil && headsInfo.IsDir() {
		_ = filepath.Walk(headsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(headsDir, path)
			if err == nil && rel != "" {
				branch := filepath.ToSlash(rel)
				if !seen[branch] {
					seen[branch] = true
					branches = append(branches, branch)
				}
			}
			return nil
		})
	}

	// 2. Parse .git/packed-refs (common dir for linked worktrees)
	packedPath := filepath.Join(commonDir, "packed-refs")
	if bytes, err := os.ReadFile(packedPath); err == nil {
		lines := strings.Split(string(bytes), "\n")
		const prefix = "refs/heads/"
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") || line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 && strings.HasPrefix(parts[1], prefix) {
				branch := strings.TrimPrefix(parts[1], prefix)
				if !seen[branch] {
					seen[branch] = true
					branches = append(branches, branch)
				}
			}
		}
	}

	return branches, nil
}

// ResolveMergedBranches returns all local branch names that have been merged into defaultBranch.
func ResolveMergedBranches(repoPath, defaultBranch string) ([]string, error) {
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	cmd := exec.Command("git", "-C", repoPath, "branch", "--merged", defaultBranch)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to query merged branches for %s: %w", defaultBranch, err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var merged []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "*")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" && trimmed != defaultBranch && !strings.Contains(trimmed, "detached") {
			merged = append(merged, trimmed)
		}
	}
	return merged, nil
}
