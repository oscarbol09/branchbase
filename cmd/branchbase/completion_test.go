package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompleteSwitchBranchesIncludesGitAndList(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	cmd := exec.Command("git", "checkout", "-b", "feature/completion")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b: %v\n%s", err, out)
	}

	writePruneSQLiteConfig(t, dir)
	got := completeSwitchBranches(dir)

	wantGit := []string{"main", "feature/completion"}
	for _, name := range wantGit {
		if !containsString(got, name) {
			t.Fatalf("completeSwitchBranches missing git branch %q; got %v", name, got)
		}
	}

	// SQLite list names the base database from the config file.
	if !containsString(got, "dev") && !containsString(got, "main") {
		t.Fatalf("completeSwitchBranches expected a managed-db name from list; got %v", got)
	}
}

func TestCompleteSwitchBranchesOutsideGitRepo(t *testing.T) {
	t.Parallel()
	got := completeSwitchBranches(t.TempDir())
	if len(got) != 0 {
		t.Fatalf("expected no branches outside a git repo, got %v", got)
	}
}

func TestRunCompletionScripts(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		"bash":       {"complete -F _branchbase branchbase", "__complete switch", "doctor", "__complete commands"},
		"zsh":        {"#compdef branchbase", "doctor", "__complete switch"},
		"fish":       {"complete -c branchbase", "doctor", "__branchbase_complete_switch"},
		"powershell": {"Register-ArgumentCompleter", "doctor", "__complete switch"},
	}

	for shell, needles := range cases {
		shell := shell
		needles := needles
		t.Run(shell, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := writeCompletionScript(&buf, []string{shell}); err != nil {
				t.Fatalf("writeCompletionScript(%s): %v", shell, err)
			}
			out := buf.String()
			for _, n := range needles {
				if !strings.Contains(out, n) {
					t.Fatalf("%s completion missing %q\n%s", shell, n, out[:min(len(out), 400)])
				}
			}
			for _, cmd := range []string{"init", "status", "proxy", "switch", "list", "prune", "tui", "hooks", "version", "doctor"} {
				if !strings.Contains(out, cmd) {
					t.Fatalf("%s completion missing command %q", shell, cmd)
				}
			}
		})
	}
}

func TestWriteCompletionUnknownShell(t *testing.T) {
	t.Parallel()
	err := writeCompletionScript(&bytes.Buffer{}, []string{"tcsh"})
	if err == nil {
		t.Fatal("expected error for unknown shell")
	}
	err = writeCompletionScript(&bytes.Buffer{}, nil)
	if err == nil {
		t.Fatal("expected error for missing shell")
	}
}

func TestRunCompleteCommands(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := writeComplete(&buf, t.TempDir(), []string{"commands"}); err != nil {
		t.Fatalf("writeComplete: %v", err)
	}
	out := buf.String()
	for _, cmd := range completionRootCommands {
		found := false
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == cmd {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("commands completion missing %q\n%s", cmd, out)
		}
	}
}

func TestRunCompleteSwitch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)
	var buf bytes.Buffer
	if err := writeComplete(&buf, dir, []string{"switch"}); err != nil {
		t.Fatalf("writeComplete: %v", err)
	}
	if !strings.Contains(buf.String(), "main") {
		t.Fatalf("switch completion missing main\n%s", buf.String())
	}
}

func TestRunCompleteHooksAndShells(t *testing.T) {
	t.Parallel()
	var hooks bytes.Buffer
	if err := writeComplete(&hooks, t.TempDir(), []string{"hooks"}); err != nil {
		t.Fatalf("hooks: %v", err)
	}
	for _, n := range []string{"install", "uninstall", "status"} {
		if !strings.Contains(hooks.String(), n) {
			t.Fatalf("hooks completion missing %q\n%s", n, hooks.String())
		}
	}
	var shells bytes.Buffer
	if err := writeComplete(&shells, t.TempDir(), []string{"shells"}); err != nil {
		t.Fatalf("shells: %v", err)
	}
	for _, n := range []string{"bash", "zsh", "fish", "powershell"} {
		if !strings.Contains(shells.String(), n) {
			t.Fatalf("shells completion missing %q\n%s", n, shells.String())
		}
	}
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestCompletionBinaryEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in short mode")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "branchbase")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	out, err := exec.Command(bin, "completion", "bash").CombinedOutput()
	if err != nil {
		t.Fatalf("completion bash: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("complete -F _branchbase branchbase")) {
		t.Fatalf("bash script missing complete hook\n%s", out)
	}

	repo := t.TempDir()
	initGitRepo(t, repo)
	cmd := exec.Command(bin, "__complete", "switch")
	cmd.Dir = repo
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("__complete switch: %v\n%s", err, got)
	}
	if !bytes.Contains(got, []byte("main")) {
		t.Fatalf("__complete switch missing main\n%s", got)
	}
}
