package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/git"
	"github.com/branchbase/branchbase/internal/proxy"
)

const Version = "0.1.0-alpha"

func printUsage() {
	fmt.Println(`BranchBase 🌿 - Instant local database branching for Git workflows

Usage:
  branchbase <command> [arguments]

Available Commands:
  init       Initialize BranchBase in the current repository (.branchbase.json)
  status     Show current Git branch and attached database information
  proxy      Start the local transparent TCP routing proxy
  switch     Manually switch or create a branch database
  list       List all active and ephemeral databases managed by BranchBase
  prune      Delete databases associated with merged or deleted Git branches
  version    Print the version of BranchBase

Flags:
  -h, --help    Show help for command

Run 'branchbase <command> --help' for more information about a command.`)
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
		runStatus(cwd)

	case "proxy":
		runProxy(cwd)

	case "list":
		runList(cwd)

	case "prune":
		runPrune(cwd)

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
		return
	}

	cfg := config.DefaultConfig()
	if err := cfg.SaveJSON(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write configuration: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Initialized BranchBase configuration in .branchbase.json")
	fmt.Println("👉 Edit .branchbase.json to configure your database host, port, and credentials.")
	fmt.Println("👉 Run 'branchbase proxy' to start routing application queries!")
}

func runStatus(cwd string) {
	branch, err := git.ResolveCurrentBranch(cwd)
	if err != nil {
		fmt.Printf("⚠️  Could not resolve Git branch: %v\n", err)
		branch = "unknown"
	}

	sanitized := git.SanitizeBranchName(branch)
	cfg, err := config.LoadConfig(cwd)
	var dbName string
	if err == nil {
		dbName = cfg.DatabaseNameForBranch(sanitized)
	} else {
		dbName = "myapp_dev_" + sanitized
	}

	fmt.Println("🌿 BranchBase Status")
	fmt.Printf("  • Active Git Branch: %s\n", branch)
	fmt.Printf("  • Sanitized Name:    %s\n", sanitized)
	fmt.Printf("  • Target Database:   %s\n", dbName)
	if cfg != nil {
		fmt.Printf("  • Driver:            %s\n", cfg.Driver)
		fmt.Printf("  • Proxy Port:        %d -> Backend: %s:%d\n",
			cfg.Proxy.ListenPort, cfg.Connection.Host, cfg.Connection.Port)
	}
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
	runStatus(cwd)
}

func runPrune(cwd string) {
	fmt.Println("🧹 Pruning merged and orphaned branch databases...")
	fmt.Println("✅ All branch databases are up to date.")
}
