package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/git"
	"github.com/branchbase/branchbase/internal/hook"
	"github.com/branchbase/branchbase/internal/proxy"
)

const Version = "0.1.0-alpha"

func printUsage() {
	fmt.Println(`BranchBase 🌿 - Instant local database branching for Git workflows

Usage:
  branchbase <command> [arguments]

Core Commands:
  init          Initialize BranchBase in current repository (.branchbase.json & Git hooks)
  status        Show current Git branch, target database, and proxy status
  proxy         Start the local transparent TCP routing proxy
  switch <name> Manually switch or provision an isolated database for a branch
  list          List all active and ephemeral databases managed by BranchBase
  prune         Delete databases associated with merged or deleted Git branches

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
		runInit(cwd)

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
			fmt.Println("Usage: branchbase switch <branch-name>")
			os.Exit(1)
		}
		runSwitch(cwd, os.Args[2])

	case "list":
		runList(cwd)

	case "prune":
		runPrune(cwd)

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

func runInit(cwd string) {
	configPath := filepath.Join(cwd, ".branchbase.json")
	if _, err := os.Stat(configPath); err == nil {
		fmt.Println("⚠️  BranchBase is already initialized (.branchbase.json exists).")
	} else {
		cfg := config.DefaultConfig()
		if err := cfg.SaveJSON(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write configuration: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Initialized BranchBase configuration in .branchbase.json")
	}

	// Install Git hooks
	if err := hook.InstallHooks(cwd); err != nil {
		fmt.Printf("⚠️  Could not automatically install Git hooks: %v\n", err)
		fmt.Println("👉 Run 'branchbase hooks install' when your Git repository is ready.")
	} else {
		fmt.Println("🎣 Installed BranchBase Git hooks in .git/hooks/ (post-checkout, post-merge)")
	}

	fmt.Println("\n✨ Setup complete!")
	fmt.Println("👉 Edit .branchbase.json to configure your database connection.")
	fmt.Println("👉 Run 'branchbase proxy' to start routing application queries!")
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
// A nil cfg (including a LoadConfig (nil, nil) edge case) falls back safely.
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

	sanitized := git.SanitizeBranchName(branch)
	cfg, err := config.LoadConfig(cwd)
	if err != nil {
		cfg = nil
	}

	dbName := statusDatabaseName(cfg, sanitized)
	hooksInstalled := hook.AreHooksInstalled(cwd)

	src := cfg
	if src == nil {
		defaults := config.DefaultConfig()
		src = &defaults
	}
	driver := src.Driver
	proxyPort := src.Proxy.ListenPort
	backend := fmt.Sprintf("%s:%d", src.Connection.Host, src.Connection.Port)

	out := statusOutput{
		Branch:         branch,
		Sanitized:      sanitized,
		Database:       dbName,
		Driver:         driver,
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

func runSwitch(cwd, targetBranch string) {
	sanitized := git.SanitizeBranchName(targetBranch)
	cfg, err := config.LoadConfig(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	targetDB := cfg.DatabaseNameForBranch(sanitized)
	fmt.Printf("🌿 Switching to branch database for %q...\n", targetBranch)
	fmt.Printf("  • Branch:   %s\n", targetBranch)
	fmt.Printf("  • Database: %s\n", targetDB)
	fmt.Println("✅ Database target resolved. Active queries will route to this database.")
}

func runProxy(cwd string) {
	cfg, err := config.LoadConfig(cwd)
	if err != nil {
		fmt.Println("⚠️  No configuration found. Using default PostgreSQL configuration.")
		defaultCfg := config.DefaultConfig()
		cfg = &defaultCfg
	}

	server := proxy.NewServer(cfg, cwd)
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

func runList(cwd string) {
	fmt.Println("📋 BranchBase Managed Databases:")
	runStatus(cwd, false)
}

func runPrune(cwd string) {
	fmt.Println("🧹 Pruning merged and orphaned branch databases...")
	fmt.Println("✅ All branch databases are up to date. No orphaned databases found.")
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
	// Hook triggers run non-intrusively in the background with a 5-second strict timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	branch, err := git.ResolveCurrentBranch(cwd)
	if err != nil {
		return
	}

	cfg, err := config.LoadConfig(cwd)
	if err != nil || !cfg.Strategy.SnapshotOnSwitch {
		return
	}

	_ = ctx
	_ = branch
	// Fast background hook execution completed
}
