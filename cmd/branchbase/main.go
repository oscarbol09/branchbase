package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/driver"
	_ "github.com/branchbase/branchbase/internal/driver/postgres"
	_ "github.com/branchbase/branchbase/internal/driver/mysql"
	_ "github.com/branchbase/branchbase/internal/driver/sqlite"
	"github.com/branchbase/branchbase/internal/compose"
	"github.com/branchbase/branchbase/internal/git"
	"github.com/branchbase/branchbase/internal/hook"
	"github.com/branchbase/branchbase/internal/proxy"
	"github.com/branchbase/branchbase/internal/tui"
)

const Version = "0.1.0-alpha"

func printUsage() {
	fmt.Println(`BranchBase 🌿 - Instant local database branching for Git workflows

Usage:
  branchbase <command> [arguments]

Core Commands:
  init [--skip-hooks]      Initialize BranchBase in current repository (.branchbase.json & Git hooks)
  status [--json]          Show current Git branch, target database, and proxy status
  proxy                    Start the local transparent TCP routing proxy
  switch <name> [--no-create] Manually switch or provision an isolated database for a branch
  list [--json]            List all active and ephemeral databases managed by BranchBase
  prune [--dry-run] [--force] Delete databases associated with merged or deleted Git branches
  tui                      Launch interactive terminal UI dashboard

Hook Management:
  hooks install    Install post-checkout and post-merge hooks into .git/hooks/
  hooks uninstall  Remove BranchBase hooks from .git/hooks/
  hooks status     Check if Git hooks are installed and active

Other:
  version       Print the version of BranchBase
  help          Show help for command

Run 'branchbase <command> --help' for more information.`)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	switch command {
	case "version", "-v", "--version":
		fmt.Printf("BranchBase v%s\n", Version)

	case "help", "-h", "--help":
		printUsage()

	case "init":
		skipHooks := false
		for _, arg := range os.Args[2:] {
			if arg == "--skip-hooks" || arg == "--no-hooks" {
				skipHooks = true
			}
		}
		if err := runInit(cwd, skipHooks); err != nil {
			os.Exit(1)
		}

	case "status":
		jsonOutput := false
		for _, arg := range os.Args[2:] {
			if arg == "--json" {
				jsonOutput = true
			}
		}
		runStatus(cwd, jsonOutput)

	case "proxy":
		runProxy(cwd)

	case "switch":
		if len(os.Args) < 3 {
			fmt.Println("Usage: branchbase switch <branch-name> [--no-create]")
			os.Exit(1)
		}
		noCreate := false
		targetBranch := ""
		for _, arg := range os.Args[2:] {
			if arg == "--no-create" {
				noCreate = true
			} else if targetBranch == "" {
				targetBranch = arg
			}
		}
		if targetBranch == "" {
			fmt.Println("Usage: branchbase switch <branch-name> [--no-create]")
			os.Exit(1)
		}
		runSwitch(cwd, targetBranch, noCreate)

	case "list":
		jsonOutput := false
		for _, arg := range os.Args[2:] {
			if arg == "--json" {
				jsonOutput = true
			}
		}
		runList(cwd, jsonOutput)

	case "tui", "dashboard", "ui":
		runTUI(cwd)

	case "prune":
		dryRun := false
		force := false
		for _, arg := range os.Args[2:] {
			switch arg {
			case "--dry-run":
				dryRun = true
			case "--force", "-f", "-y":
				force = true
			}
		}
		runPrune(cwd, dryRun, force)

	case "hooks":
		subcmd := "status"
		if len(os.Args) >= 3 {
			subcmd = os.Args[2]
		}
		runHooks(cwd, subcmd)

	case "hook-trigger":
		runHookTrigger(cwd, os.Args[2:])

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %q\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func runInit(cwd string, skipHooks bool) error {
	configPath := filepath.Join(cwd, ".branchbase.json")
	if _, err := os.Stat(configPath); err == nil {
		fmt.Println("⚠️  BranchBase is already initialized (.branchbase.json exists).")
	} else {
		cfg := config.DefaultConfig()

		// Detect Docker Compose service
		if dbSvc, composeFile, err := compose.DetectCompose(cwd); err == nil && dbSvc != nil {
			compose.ApplyToConfig(&cfg, dbSvc)
			fmt.Printf("🐳 Auto-detected %s database from %s (port %d, db %q)\n", dbSvc.Driver, composeFile, dbSvc.Port, dbSvc.Database)
		}

		if err := cfg.SaveJSON(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write configuration: %v\n", err)
			return err
		}
		fmt.Println("✅ Initialized BranchBase configuration in .branchbase.json")
	}

	if skipHooks {
		fmt.Println("⚪ Git hooks installation skipped (--skip-hooks specified).")
		fmt.Println("\n✨ Setup complete!")
		fmt.Println("👉 Edit .branchbase.json to configure your database connection.")
		fmt.Println("👉 Run 'branchbase proxy' to start routing application queries!")
		return nil
	}

	// Install Git hooks
	if err := hook.InstallHooks(cwd); err != nil {
		fmt.Printf("⚠️  Could not automatically install Git hooks: %v\n", err)
		fmt.Println("👉 Run 'branchbase hooks install' when your Git repository is ready.")
		fmt.Println("\n⚠️  Partial setup completed (Git hooks not installed).")
		return err
	}

	fmt.Println("🎣 Installed BranchBase Git hooks in .git/hooks/ (post-checkout, post-merge)")
	fmt.Println("\n✨ Setup complete!")
	fmt.Println("👉 Edit .branchbase.json to configure your database connection.")
	fmt.Println("👉 Run 'branchbase proxy' to start routing application queries!")
	return nil
}

// statusOutput is the JSON shape for `branchbase status --json`.
type statusOutput struct {
	Branch         string `json:"branch"`
	Sanitized      string `json:"sanitized"`
	Database       string `json:"database"`
	Driver         string `json:"driver"`
	ProxyPort      int    `json:"proxy_port"`
	Backend        string `json:"backend"`
	HooksInstalled bool   `json:"hooks_installed"`
}

// statusDatabaseName resolves the branch database name for status output.
func statusDatabaseName(cfg *config.Config, sanitized string) string {
	if cfg == nil {
		return "myapp_dev_" + sanitized
	}
	return cfg.DatabaseNameForBranch(sanitized)
}

// buildStatus gathers status fields with graceful defaults when config is missing.
func buildStatus(cwd string) (statusOutput, error) {
	branch, err := git.ResolveCurrentBranch(cwd)
	branchErr := err
	if err != nil {
		branch = "unknown"
	}

	cfg, _ := config.LoadConfig(cwd)
	sanitized := git.SanitizeBranchName(branch)

	driverName := "postgres"
	if cfg != nil && cfg.Driver != "" {
		driverName = cfg.Driver
	}

	proxyPort := 5432
	if cfg != nil && cfg.Proxy.ListenPort > 0 {
		proxyPort = cfg.Proxy.ListenPort
	}

	backend := "127.0.0.1:5433"
	if cfg != nil {
		backend = fmt.Sprintf("%s:%d", cfg.Connection.Host, cfg.Connection.Port)
	}

	hooksInstalled := hook.AreHooksInstalled(cwd)

	out := statusOutput{
		Branch:         branch,
		Sanitized:      sanitized,
		Database:       statusDatabaseName(cfg, sanitized),
		Driver:         driverName,
		ProxyPort:      proxyPort,
		Backend:        backend,
		HooksInstalled: hooksInstalled,
	}
	return out, branchErr
}

func runStatus(cwd string, jsonOutput bool) {
	info, branchErr := buildStatus(cwd)

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(info); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode status JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if branchErr != nil {
		fmt.Printf("⚠️  Could not resolve Git branch: %v\n", branchErr)
	}

	fmt.Println("🌿 BranchBase Status")
	fmt.Printf("  • Active Git Branch: %s\n", info.Branch)
	fmt.Printf("  • Sanitized Name:    %s\n", info.Sanitized)
	fmt.Printf("  • Target Database:   %s\n", info.Database)
	fmt.Printf("  • Driver:            %s\n", info.Driver)
	fmt.Printf("  • Proxy Port:        %d -> Backend: %s\n", info.ProxyPort, info.Backend)
	if info.HooksInstalled {
		fmt.Println("  • Git Hooks:         ✅ Active (.git/hooks/post-checkout)")
	} else {
		fmt.Println("  • Git Hooks:         ⚪ Not installed (run 'branchbase hooks install')")
	}
}

// formatBytes formats bytes into human-readable representations.
func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// driverParamsForConfig converts application configuration into driver parameters.
func driverParamsForConfig(cfg *config.Config, lightweight bool) map[string]interface{} {
	params := make(map[string]interface{})
	params["host"] = cfg.Connection.Host
	params["port"] = cfg.Connection.Port
	params["user"] = cfg.Connection.User
	params["password"] = cfg.Connection.Password
	params["base_database"] = cfg.Connection.BaseDatabase
	params["sslmode"] = "disable"

	if lightweight {
		params["max_open_conns"] = 1
		params["max_idle_conns"] = 1
	}

	// For SQLite
	if cfg.Connection.Path != "" {
		params["path"] = cfg.Connection.Path
	} else if cfg.Connection.BaseDatabase != "" {
		params["path"] = cfg.Connection.BaseDatabase
	}

	return params
}

// getDriverForConfig resolves and initializes a database driver instance from configuration.
func getDriverForConfig(cfg *config.Config, lightweight bool) (driver.Driver, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is nil")
	}

	drvName := cfg.Driver
	if drvName == "" {
		drvName = "postgres"
	}

	return driver.GetDriver(drvName, driverParamsForConfig(cfg, lightweight))
}

func runTUI(cwd string) {
	cfg, err := config.LoadConfig(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	drv, err := getDriverForConfig(cfg, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing driver %s: %v\n", cfg.Driver, err)
		os.Exit(1)
	}
	defer func() {
		_ = drv.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	if err := tui.Run(ctx, cwd, cfg, drv, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}

func runSwitch(cwd, targetBranch string, noCreate bool) {
	sanitized := git.SanitizeBranchName(targetBranch)
	cfg, err := config.LoadConfig(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	targetDB := cfg.DatabaseNameForBranch(sanitized)
	defaultBranch := cfg.Proxy.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	fmt.Printf("🌿 Switching to branch database for %q...\n", targetBranch)
	fmt.Printf("  • Branch:   %s\n", targetBranch)
	fmt.Printf("  • Database: %s\n", targetDB)

	if noCreate {
		fmt.Println("✅ Database target resolved (--no-create specified).")
		return
	}

	drv, err := getDriverForConfig(cfg, false)
	if err != nil {
		fmt.Printf("⚠️  Could not connect to database driver: %v\n", err)
		fmt.Println("✅ Target resolved. Database provisioning skipped.")
		return
	}
	defer func() {
		_ = drv.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exists, err := drv.BranchExists(ctx, sanitized)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Error checking branch existence: %v\n", err)
		return
	}

	if exists {
		fmt.Println("✅ Database already exists. Active queries will route to this database.")
		return
	}

	fmt.Printf("🪄 Provisioning branch database %q from base %q...\n", targetDB, defaultBranch)
	if err := drv.CreateBranch(ctx, defaultBranch, sanitized); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to create branch database: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Branch database provisioned successfully. Active queries will route to this database.")
}

func runProxy(cwd string) {
	cfg, err := config.LoadConfig(cwd)
	if err != nil {
		fmt.Println("⚠️  No configuration found. Using default PostgreSQL configuration.")
		defaultCfg := config.DefaultConfig()
		cfg = &defaultCfg
	}

	drv, err := getDriverForConfig(cfg, false)
	if err != nil {
		fmt.Printf("⚠️  Could not initialize driver for %s: %v (JIT provisioning disabled)\n", cfg.Driver, err)
	} else {
		defer func() {
			_ = drv.Close()
		}()
	}

	server := proxy.NewServer(cfg, cwd, drv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start proxy: %v\n", err)
		os.Exit(1)
	}

	// Wait for interrupt signal (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down BranchBase proxy...")
	_ = server.Stop()
}

func runList(cwd string, jsonOutput bool) {
	cfg, err := config.LoadConfig(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	drv, err := getDriverForConfig(cfg, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing driver %s: %v\n", cfg.Driver, err)
		os.Exit(1)
	}
	defer func() {
		_ = drv.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	branches, err := drv.ListBranches(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error querying branch databases: %v\n", err)
		os.Exit(1)
	}

	activeBranch, _ := git.ResolveCurrentBranch(cwd)
	sanitizedActive := git.SanitizeBranchName(activeBranch)

	for i := range branches {
		if git.SanitizeBranchName(branches[i].Name) == sanitizedActive {
			branches[i].IsActive = true
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(branches); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Println("📋 BranchBase Managed Databases:")
	if len(branches) == 0 {
		fmt.Println("  No databases currently managed by BranchBase.")
		return
	}

	fmt.Printf("  %-3s %-24s %-28s %-12s %-10s\n", "", "BRANCH", "DATABASE", "SIZE", "STATUS")
	fmt.Printf("  %-3s %-24s %-28s %-12s %-10s\n", "", "------", "--------", "----", "------")

	for _, b := range branches {
		marker := " "
		status := "Idle"
		if b.IsProtected {
			status = "Protected"
		}
		if b.IsActive {
			marker = "*"
			status = "Active"
		}

		fmt.Printf("  %-3s %-24s %-28s %-12s %-10s\n",
			marker,
			b.Name,
			b.Database,
			formatBytes(b.SizeBytes),
			status,
		)
	}

	fmt.Printf("\nTotal: %d managed database(s)\n", len(branches))
}

type pruneCandidate struct {
	branch driver.BranchInfo
	reason string
}

func runPrune(cwd string, dryRun, force bool) {
	fmt.Println("🧹 Checking for merged and orphaned branch databases...")

	cfg, err := config.LoadConfig(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	drv, err := getDriverForConfig(cfg, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing driver %s: %v\n", cfg.Driver, err)
		os.Exit(1)
	}
	defer func() {
		_ = drv.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	branches, err := drv.ListBranches(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing databases: %v\n", err)
		os.Exit(1)
	}

	defaultBranch := cfg.Proxy.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	activeBranch, err := git.ResolveCurrentBranch(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to resolve active Git branch: %v\n", err)
		os.Exit(1)
	}
	sanitizedActive := git.SanitizeBranchName(activeBranch)

	localBranches, err := git.ResolveLocalBranches(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to resolve local Git branches: %v\n", err)
		os.Exit(1)
	}
	localMap := make(map[string]bool)
	for _, lb := range localBranches {
		localMap[lb] = true
		localMap[git.SanitizeBranchName(lb)] = true
	}

	mergedBranches, err := git.ResolveMergedBranches(cwd, defaultBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to resolve merged Git branches: %v\n", err)
		os.Exit(1)
	}
	mergedMap := make(map[string]bool)
	for _, mb := range mergedBranches {
		mergedMap[mb] = true
		mergedMap[git.SanitizeBranchName(mb)] = true
	}

	var candidates []pruneCandidate
	var totalBytes int64

	for _, b := range branches {
		// Never prune protected base database or default branch or currently active branch
		if b.IsProtected || b.Name == defaultBranch || b.Database == cfg.Connection.BaseDatabase {
			continue
		}
		sanitizedName := git.SanitizeBranchName(b.Name)
		if sanitizedName == sanitizedActive || sanitizedName == git.SanitizeBranchName(defaultBranch) {
			continue
		}

		if mergedMap[b.Name] || mergedMap[sanitizedName] {
			candidates = append(candidates, pruneCandidate{branch: b, reason: "merged"})
			totalBytes += b.SizeBytes
		} else if len(localMap) > 0 && !localMap[b.Name] && !localMap[sanitizedName] {
			candidates = append(candidates, pruneCandidate{branch: b, reason: "orphaned"})
			totalBytes += b.SizeBytes
		}
	}

	if len(candidates) == 0 {
		fmt.Println("✅ All branch databases are up to date. No orphaned or merged databases found.")
		return
	}

	fmt.Printf("\nFound %d candidate database(s) to prune:\n", len(candidates))
	for _, c := range candidates {
		fmt.Printf("  • %-26s (branch: %-16s [%s]) - %s\n",
			c.branch.Database, c.branch.Name, c.reason, formatBytes(c.branch.SizeBytes))
	}
	fmt.Printf("Total disk space to free: %s\n\n", formatBytes(totalBytes))

	if dryRun {
		fmt.Println("🔍 Dry run complete. No databases were deleted.")
		return
	}

	if !force {
		fmt.Print("Are you sure you want to delete these databases? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("\nAborted. No databases were deleted.")
			return
		}
		trimmed := strings.ToLower(strings.TrimSpace(input))
		if trimmed != "y" && trimmed != "yes" {
			fmt.Println("Aborted. No databases were deleted.")
			return
		}
	}

	for _, c := range candidates {
		if err := drv.DeleteBranch(ctx, c.branch.Name); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠️ Failed to delete %s: %v\n", c.branch.Database, err)
		} else {
			fmt.Printf("  🗑️  Deleted %s\n", c.branch.Database)
		}
	}

	fmt.Println("✅ Pruning complete.")
}

func runHooks(cwd, action string) {
	switch action {
	case "install":
		if err := hook.InstallHooks(cwd); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to install hooks: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ BranchBase Git hooks installed successfully in .git/hooks/")

	case "uninstall":
		if err := hook.UninstallHooks(cwd); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to uninstall hooks: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ BranchBase Git hooks removed from .git/hooks/")

	case "status":
		if hook.AreHooksInstalled(cwd) {
			fmt.Println("✅ BranchBase Git hooks are installed and active.")
		} else {
			fmt.Println("⚪ BranchBase Git hooks are not installed. Run 'branchbase hooks install'.")
		}

	default:
		fmt.Printf("Unknown hooks action %q. Use: install, uninstall, or status.\n", action)
		os.Exit(1)
	}
}

func runHookTrigger(cwd string, args []string) {
	// For post-checkout hooks: Git passes <previous_head> <new_head> <flag>.
	// flag == "1" indicates a branch switch; flag == "0" indicates a single-file checkout.
	// Skip execution on file checkouts to avoid unnecessary database operations.
	if len(args) >= 4 && args[0] == "post-checkout" && args[3] == "0" {
		return
	}
	if len(args) == 3 && args[2] == "0" {
		return
	}

	// Hook triggers run non-intrusively in the background with a 5-second strict timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := config.LoadConfig(cwd)
	if err != nil || !cfg.Strategy.SnapshotOnSwitch {
		return
	}


	branch, err := git.ResolveCurrentBranch(cwd)
	if err != nil || branch == "" {
		return
	}

	defaultBranch := cfg.Proxy.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	sanitized := git.SanitizeBranchName(branch)
	if sanitized == git.SanitizeBranchName(defaultBranch) {
		return
	}

	drv, err := getDriverForConfig(cfg, true)
	if err != nil {
		logHookError(cwd, fmt.Sprintf("failed to get driver for config: %v", err))
		return
	}
	defer func() {
		_ = drv.Close()
	}()

	exists, err := drv.BranchExists(ctx, sanitized)
	if err != nil {
		logHookError(cwd, fmt.Sprintf("failed to check branch existence for %q: %v", sanitized, err))
		return
	}
	if exists {
		return
	}

	if err := drv.CreateBranch(ctx, defaultBranch, sanitized); err != nil {
		logHookError(cwd, fmt.Sprintf("failed to pre-warm branch %q: %v", sanitized, err))
	}
}

func logHookError(cwd, msg string) {
	logLine := fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), msg)
	f, err := os.OpenFile(filepath.Join(cwd, ".branchbase.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = f.WriteString(logLine)
		_ = f.Close()
	}
}
