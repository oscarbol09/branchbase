package hook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndUninstallHooks(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "branchbase_hook_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	gitDir := filepath.Join(tempDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git: %v", err)
	}

	// 1. Install
	if err := InstallHooks(tempDir); err != nil {
		t.Fatalf("InstallHooks failed: %v", err)
	}

	if !AreHooksInstalled(tempDir) {
		t.Errorf("expected hooks to be installed")
	}

	// 2. Re-install should be idempotent
	if err := InstallHooks(tempDir); err != nil {
		t.Fatalf("Idempotent InstallHooks failed: %v", err)
	}

	// 3. Uninstall
	if err := UninstallHooks(tempDir); err != nil {
		t.Fatalf("UninstallHooks failed: %v", err)
	}

	if AreHooksInstalled(tempDir) {
		t.Errorf("expected hooks to be uninstalled")
	}
}
