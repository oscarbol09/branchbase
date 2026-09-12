package hook

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUninstallHooks_ReversedMarkers is the regression test for issue #12.
//
// Before the fix, UninstallHooks would panic with "slice bounds out of range"
// when the hook file contained markers in reverse order (end before start),
// because startIdx >= endIdx after adding len(hookMarkerEnd).
//
// After the fix, the function detects the condition and skips the file with a
// warning instead of panicking.
func TestUninstallHooks_ReversedMarkers(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a hook with markers in reverse order (end before start).
	corrupted := "#!/bin/sh\n" +
		hookMarkerEnd + "\n" +
		"# some code\n" +
		hookMarkerStart + "\n"
	hookPath := filepath.Join(hooksDir, "post-checkout")
	if err := os.WriteFile(hookPath, []byte(corrupted), 0755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	// Must not panic.
	if err := UninstallHooks(tmpDir); err != nil {
		t.Fatalf("UninstallHooks returned error: %v", err)
	}

	// The corrupted file should be left intact (we skipped it).
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("hook file missing after uninstall: %v", err)
	}
	if string(data) != corrupted {
		t.Errorf("expected corrupted hook to be left unchanged, got:\n%s", data)
	}
}

// TestUninstallHooks_TruncatedEndMarker covers the case where the end marker
// is present but the string ends before the full marker — endIdx + len would
// exceed len(content) without the bounds check.
func TestUninstallHooks_TruncatedEndMarker(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Craft content where endIdx + len(hookMarkerEnd) would exceed the string.
	// Use hookMarkerStart normally, then end with a prefix of hookMarkerEnd.
	content := "#!/bin/sh\n" +
		hookMarkerStart + "\n" +
		"# code\n" +
		hookMarkerEnd[:5] // truncated end marker
	hookPath := filepath.Join(hooksDir, "post-checkout")
	if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	// UninstallHooks won't find hookMarkerEnd (prefix != full marker), so the
	// if-block is simply skipped. No panic either way — validate that.
	if err := UninstallHooks(tmpDir); err != nil {
		t.Fatalf("UninstallHooks returned error: %v", err)
	}
}

// TestUninstallHooks_NormalRoundtrip verifies that well-formed markers still
// uninstall correctly after the bounds-check guards were added.
func TestUninstallHooks_NormalRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := InstallHooks(tmpDir); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	if !AreHooksInstalled(tmpDir) {
		t.Fatal("hooks not installed")
	}

	if err := UninstallHooks(tmpDir); err != nil {
		t.Fatalf("UninstallHooks: %v", err)
	}
	if AreHooksInstalled(tmpDir) {
		t.Error("hooks still reported as installed after uninstall")
	}
}
