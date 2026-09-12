package hook

import (
	"os"
	"path/filepath"
	"strings"
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

func TestUninstallHooksCorruptedMarkers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "branchbase_hook_corrupt")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	hooksDir := filepath.Join(tempDir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("mkdir hooks: %v", err)
	}

	// Reversed markers: end before start
	corrupted := "#!/bin/sh\n" + hookMarkerEnd + "\necho hi\n" + hookMarkerStart + "\n"
	hookFile := filepath.Join(hooksDir, "post-checkout")
	if err := os.WriteFile(hookFile, []byte(corrupted), 0755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	err = UninstallHooks(tempDir)
	if err == nil {
		t.Fatalf("expected error for corrupted markers, got nil")
	}
	if !strings.Contains(err.Error(), "corrupted branchbase hook markers") {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should be left alone
	data, readErr := os.ReadFile(hookFile)
	if readErr != nil {
		t.Fatalf("hook file should remain: %v", readErr)
	}
	if string(data) != corrupted {
		t.Fatalf("hook file was modified despite corruption")
	}
}
