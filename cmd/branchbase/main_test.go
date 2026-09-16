package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bytes"
	"context"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/tui"
)

func TestStatusDatabaseNameNilConfig(t *testing.T) {
	t.Parallel()
	got := statusDatabaseName(nil, "feature-x")
	want := "myapp_dev_feature-x"
	if got != want {
		t.Fatalf("statusDatabaseName(nil, feature-x) = %q, want %q", got, want)
	}
}

func TestDriverParamsForConfig(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	for name, lightweight := range map[string]bool{
		"default":     false,
		"lightweight": true,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			params := driverParamsForConfig(&cfg, lightweight)
			if got := params["host"]; got != cfg.Connection.Host {
				t.Fatalf("host = %v, want %v", got, cfg.Connection.Host)
			}

			if lightweight {
				if got := params["max_open_conns"]; got != 1 {
					t.Fatalf("max_open_conns = %v, want 1", got)
				}
				if got := params["max_idle_conns"]; got != 1 {
					t.Fatalf("max_idle_conns = %v, want 1", got)
				}
				return
			}

			if _, ok := params["max_open_conns"]; ok {
				t.Fatal("default params must not override max_open_conns")
			}
			if _, ok := params["max_idle_conns"]; ok {
				t.Fatal("default params must not override max_idle_conns")
			}
		})
	}
}

func TestStatusDatabaseNameUsesConfig(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	got := statusDatabaseName(&cfg, "feature-x")
	want := cfg.DatabaseNameForBranch("feature-x")
	if got != want {
		t.Fatalf("statusDatabaseName(&cfg, feature-x) = %q, want %q", got, want)
	}
}

func TestStatusOutputJSONMarshal(t *testing.T) {
	t.Parallel()
	info := statusOutput{
		Branch:         "main",
		Sanitized:      "main",
		Database:       "myapp_dev_main",
		Driver:         "postgres",
		ProxyPort:      5432,
		Backend:        "127.0.0.1:5433",
		HooksInstalled: true,
	}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"branch", "sanitized", "database", "driver", "proxy_port", "backend", "hooks_installed"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q in JSON: %s", key, raw)
		}
	}
}

func TestBuildStatusMissingConfigDefaults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Not a git repo and no config — should still return graceful defaults, not panic.
	info, _ := buildStatus(dir)
	if info.Branch != "unknown" {
		t.Fatalf("branch = %q, want unknown", info.Branch)
	}
	if info.Driver != "postgres" {
		t.Fatalf("driver = %q, want postgres", info.Driver)
	}
	if info.Database == "" {
		t.Fatal("database should have a fallback value")
	}
	raw, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("invalid JSON: %s", raw)
	}
}

func TestBuildStatusWithConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Connection.BaseDatabase = "myapp_dev"
	cfg.Proxy.ListenPort = 5433
	cfg.Connection.Host = "127.0.0.1"
	cfg.Connection.Port = 5432
	if err := cfg.SaveJSON(filepath.Join(dir, ".branchbase.json")); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}
	// init a git repo on main so ResolveCurrentBranch works
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	// Without a real git repo ResolveCurrentBranch may fail — that's ok; focus on config fields.
	info, _ := buildStatus(dir)
	if info.Driver != "postgres" {
		t.Fatalf("driver = %q", info.Driver)
	}
	if info.ProxyPort != 5433 {
		t.Fatalf("proxy_port = %d, want 5433", info.ProxyPort)
	}
	if info.Backend != "127.0.0.1:5432" {
		t.Fatalf("backend = %q, want 127.0.0.1:5432", info.Backend)
	}
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestRunSwitchAndListWithSqlite(t *testing.T) {
	dir := t.TempDir()

	// 1. Create base sqlite database
	baseDB := filepath.Join(dir, "app_dev.db")
	if err := os.WriteFile(baseDB, []byte("SQLite format 3\x00base-content"), 0644); err != nil {
		t.Fatalf("failed to create base db: %v", err)
	}

	// 2. Write config pointing to sqlite
	cfg := config.DefaultConfig()
	cfg.Driver = "sqlite"
	cfg.Connection.BaseDatabase = baseDB
	cfg.Connection.Path = baseDB
	cfg.Proxy.DefaultBranch = "main"

	if err := cfg.SaveJSON(filepath.Join(dir, ".branchbase.json")); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}

	// 3. runSwitch with --no-create
	runSwitch(dir, "feature/dry-test", true)
	branchDBPath := filepath.Join(dir, "app_dev_feature_dry_test.db")
	if _, err := os.Stat(branchDBPath); !os.IsNotExist(err) {
		t.Fatalf("expected %s not to exist after switch --no-create", branchDBPath)
	}

	// 4. runSwitch normal (should provision database)
	runSwitch(dir, "feature/auth-v1", false)
	authDBPath := filepath.Join(dir, "app_dev_feature_auth_v1.db")
	if _, err := os.Stat(authDBPath); err != nil {
		t.Fatalf("expected %s to exist after switch, got err: %v", authDBPath, err)
	}

	// 5. runSwitch again (idempotent, already exists)
	runSwitch(dir, "feature/auth-v1", false)

	// 6. runList (both table and json)
	runList(dir, false)
	runList(dir, true)
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	commands := [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "BranchBase Tests"},
		{"config", "user.email", "branchbase-tests@example.invalid"},
		{"commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range commands {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
}

func writePruneSQLiteConfig(t *testing.T, dir string) {
	t.Helper()

	baseDB := filepath.Join(dir, "dev.db")
	if err := os.WriteFile(baseDB, []byte("base-db-content"), 0644); err != nil {
		t.Fatalf("write base db: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Driver = "sqlite"
	cfg.Connection.BaseDatabase = baseDB
	cfg.Connection.Path = baseDB
	cfg.Proxy.DefaultBranch = "main"
	if err := cfg.SaveJSON(filepath.Join(dir, ".branchbase.json")); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func runPruneFailureHelper(t *testing.T, dir string) (string, int) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunPruneGitResolutionFailureHelper$")
	cmd.Env = append(os.Environ(),
		"BRANCHBASE_PRUNE_HELPER=1",
		"BRANCHBASE_PRUNE_DIR="+dir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run prune helper: %v", err)
	}
	return string(output), exitErr.ExitCode()
}

func TestRunPruneGitResolutionFailureHelper(t *testing.T) {
	if os.Getenv("BRANCHBASE_PRUNE_HELPER") != "1" {
		return
	}
	runPrune(os.Getenv("BRANCHBASE_PRUNE_DIR"), true, false)
}

func TestRunPruneFailsWhenActiveBranchCannotBeResolved(t *testing.T) {
	dir := t.TempDir()
	writePruneSQLiteConfig(t, dir)

	// Case 1: No .git directory at all
	output, exitCode := runPruneFailureHelper(t, dir)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\noutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "Failed to resolve active Git branch") {
		t.Fatalf("missing active branch resolution error:\n%s", output)
	}
	if strings.Contains(output, "All branch databases are up to date") {
		t.Fatalf("prune reported a false success after Git resolution failed:\n%s", output)
	}

	// Case 2: Detached HEAD state
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	// Write 40-character raw commit hash representing detached HEAD
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("0123456789abcdef0123456789abcdef01234567\n"), 0644); err != nil {
		t.Fatalf("write detached HEAD: %v", err)
	}

	outputDetached, exitCodeDetached := runPruneFailureHelper(t, dir)
	if exitCodeDetached != 1 {
		t.Fatalf("detached HEAD exit code = %d, want 1\noutput:\n%s", exitCodeDetached, outputDetached)
	}
	if !strings.Contains(outputDetached, "Failed to resolve active Git branch") {
		t.Fatalf("missing active branch resolution error for detached HEAD:\n%s", outputDetached)
	}
}

func TestRunPruneFailsWhenMergedBranchesCannotBeResolved(t *testing.T) {
	dir := t.TempDir()
	writePruneSQLiteConfig(t, dir)

	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	// Keep a local main ref discoverable by the filesystem fallback, but make it
	// invalid so `git branch --merged main` fails deterministically.
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte("not-a-commit\n"), 0644); err != nil {
		t.Fatalf("write main ref: %v", err)
	}

	output, exitCode := runPruneFailureHelper(t, dir)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\noutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "Failed to resolve merged Git branches") {
		t.Fatalf("missing merged branch resolution error:\n%s", output)
	}
	if strings.Contains(output, "All branch databases are up to date") {
		t.Fatalf("prune reported a false success after Git resolution failed:\n%s", output)
	}
}

func TestRunPruneWithSqlite(t *testing.T) {
	dir := t.TempDir()

	// 1. Initialize a real repository so merged-branch resolution is meaningful.
	initGitRepo(t, dir)

	baseDB := filepath.Join(dir, "dev.db")
	_ = os.WriteFile(baseDB, []byte("base-db-content"), 0644)

	cfg := config.DefaultConfig()
	cfg.Driver = "sqlite"
	cfg.Connection.BaseDatabase = baseDB
	cfg.Connection.Path = baseDB
	cfg.Proxy.DefaultBranch = "main"
	_ = cfg.SaveJSON(filepath.Join(dir, ".branchbase.json"))

	// Create an orphaned branch database (no git branch exists for it)
	orphanedDB := filepath.Join(dir, "dev_orphaned_feat.db")
	_ = os.WriteFile(orphanedDB, []byte("orphaned-content"), 0644)

	// 2. runPrune dry-run: should not delete
	runPrune(dir, true, false)
	if _, err := os.Stat(orphanedDB); err != nil {
		t.Fatalf("dry run must not delete database: %v", err)
	}

	// 3. runPrune force: should delete orphaned database
	runPrune(dir, false, true)
	if _, err := os.Stat(orphanedDB); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be deleted after force prune", orphanedDB)
	}
}

func TestRunHookTriggerWithSqlite(t *testing.T) {
	dir := t.TempDir()

	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/hook-prewarm\n"), 0644)

	baseDB := filepath.Join(dir, "myapp.db")
	_ = os.WriteFile(baseDB, []byte("myapp-content"), 0644)

	cfg := config.DefaultConfig()
	cfg.Driver = "sqlite"
	cfg.Connection.BaseDatabase = baseDB
	cfg.Connection.Path = baseDB
	cfg.Strategy.SnapshotOnSwitch = true
	cfg.Proxy.DefaultBranch = "main"
	_ = cfg.SaveJSON(filepath.Join(dir, ".branchbase.json"))

	runHookTrigger(dir, nil)

	prewarmedDB := filepath.Join(dir, "myapp_feature_hook_prewarm.db")
	if _, err := os.Stat(prewarmedDB); err != nil {
		t.Fatalf("expected pre-warmed database %s to exist: %v", prewarmedDB, err)
	}
}

func TestRunHookTriggerFileCheckoutSkipped(t *testing.T) {
	dir := t.TempDir()

	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/file-checkout\n"), 0644)

	baseDB := filepath.Join(dir, "myapp.db")
	_ = os.WriteFile(baseDB, []byte("myapp-content"), 0644)

	cfg := config.DefaultConfig()
	cfg.Driver = "sqlite"
	cfg.Connection.BaseDatabase = baseDB
	cfg.Connection.Path = baseDB
	cfg.Strategy.SnapshotOnSwitch = true
	cfg.Proxy.DefaultBranch = "main"
	_ = cfg.SaveJSON(filepath.Join(dir, ".branchbase.json"))

	// Post-checkout with flag "0" (single file checkout) should skip prewarming
	argsWithHookName := []string{"post-checkout", "HEAD~1", "HEAD", "0"}
	runHookTrigger(dir, argsWithHookName)

	targetDB := filepath.Join(dir, "myapp_feature_file_checkout.db")
	if _, err := os.Stat(targetDB); !os.IsNotExist(err) {
		t.Fatalf("expected database %s not to be created on file checkout flag '0'", targetDB)
	}

	// Direct 3 args with flag "0" should also skip
	argsDirect := []string{"HEAD~1", "HEAD", "0"}
	runHookTrigger(dir, argsDirect)

	if _, err := os.Stat(targetDB); !os.IsNotExist(err) {
		t.Fatalf("expected database %s not to be created on direct flag '0'", targetDB)
	}
}

func TestRunTUIWithSqlite(t *testing.T) {
	dir := t.TempDir()

	baseDB := filepath.Join(dir, "app_tui.db")
	if err := os.WriteFile(baseDB, []byte("SQLite format 3\x00tui-content"), 0644); err != nil {
		t.Fatalf("failed to create base db: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Driver = "sqlite"
	cfg.Connection.BaseDatabase = baseDB
	cfg.Connection.Path = baseDB
	cfg.Proxy.DefaultBranch = "main"

	if err := cfg.SaveJSON(filepath.Join(dir, ".branchbase.json")); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}

	drv, err := getDriverForConfig(&cfg, false)
	if err != nil {
		t.Fatalf("getDriverForConfig: %v", err)
	}
	defer drv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Simulate user typing 'q' immediately to exit TUI
	input := strings.NewReader("q\n")
	var output bytes.Buffer

	if err := tui.Run(ctx, dir, &cfg, drv, input, &output); err != nil {
		t.Fatalf("tui.Run failed: %v", err)
	}

	outStr := output.String()
	if !strings.Contains(outStr, "BranchBase Dashboard") {
		t.Fatalf("expected output to contain dashboard header, got: %s", outStr)
	}
}
