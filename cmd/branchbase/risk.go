package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/git"
	"github.com/branchbase/branchbase/internal/risk"
)

func runRisk(cwd string, args []string) {
	if len(args) == 0 {
		printRiskUsage()
		os.Exit(2)
	}

	subcommand := args[0]
	switch subcommand {
	case "analyze":
		if len(args) < 2 {
			fmt.Println("Usage: branchbase risk analyze <migration.sql> [--json] [--offline] [--github]")
			os.Exit(2)
		}
		filePath := args[1]
		jsonOutput := false
		githubOutput := false
		offline := false
		for _, a := range args[2:] {
			if a == "--json" {
				jsonOutput = true
			} else if a == "--github" {
				githubOutput = true
			} else if a == "--offline" {
				offline = true
			}
		}
		runRiskAnalyze(cwd, filePath, jsonOutput, githubOutput, offline)

	case "check":
		policyPath := ""
		branchName := ""
		force := false
		offline := false
		jsonOutput := false
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--policy":
				if i+1 < len(args) {
					policyPath = args[i+1]
					i++
				}
			case "--branch":
				if i+1 < len(args) {
					branchName = args[i+1]
					i++
				}
			case "--force", "-f":
				force = true
			case "--offline":
				offline = true
			case "--json":
				jsonOutput = true
			}
		}
		runRiskCheck(cwd, branchName, policyPath, force, offline, jsonOutput)

	default:
		fmt.Fprintf(os.Stderr, "Unknown risk subcommand %q\n\n", subcommand)
		printRiskUsage()
		os.Exit(2)
	}
}

func printRiskUsage() {
	fmt.Println(`BranchBase Risk Gate 🛡️ — Automated database migration risk analysis

Usage:
  branchbase risk <command> [arguments]

Commands:
  analyze <file.sql>   Parse and analyze a migration file for potential risks
  check [--branch <b>] Evaluate pending migrations against .branchbase/risk-policy.yml

Flags:
  --json       Output result as structured JSON
  --github     Output result as GitHub Action annotation commands
  --offline    Force local heuristic analysis without remote API
  --force      Bypass blocking policy actions (proceed with caution)
  --policy <p> Path to custom risk-policy.yml`)
}

func resolveIntrospector(cfg *config.Config, schemaBranch string) (risk.SchemaIntrospector, func()) {
	if cfg == nil {
		return risk.NewMockIntrospector(), func() {}
	}
	if schemaBranch == "" {
		schemaBranch = cfg.Proxy.DefaultBranch
	}
	if schemaBranch == "" {
		schemaBranch = "main"
	}

	drv, err := getDriverForConfig(cfg, true)
	if err != nil {
		return risk.NewMockIntrospector(), func() {}
	}

	if drv.Name() == "postgres" {
		if pgDrv, ok := drv.(interface{ OpenDatabase(string) (*sql.DB, error) }); ok {
			databaseName := cfg.DatabaseNameForBranch(git.SanitizeBranchName(schemaBranch))
			db, err := pgDrv.OpenDatabase(databaseName)
			if err := drv.Close(); err != nil { fmt.Fprintf(os.Stderr, "failed to close drv: %v\n", err) }
			if err == nil {
				return risk.NewPostgresIntrospector(db), func() { if err := db.Close(); err != nil { fmt.Fprintf(os.Stderr, "failed to close db: %v\n", err) } }
			}
		}
	}

	if err := drv.Close(); err != nil { fmt.Fprintf(os.Stderr, "failed to close drv: %v\n", err) }
	return risk.NewMockIntrospector(), func() {}
}

func changedMigrationFiles(repoRoot, defaultBranch string) ([]string, error) {
	if strings.TrimSpace(defaultBranch) == "" {
		defaultBranch = "main"
	}
	var baseRef string
	for _, candidate := range []string{defaultBranch, "origin/" + defaultBranch} {
		if err := exec.Command("git", "-C", repoRoot, "rev-parse", "--verify", candidate+"^{commit}").Run(); err == nil {
			baseRef = candidate
			break
		}
	}
	if baseRef == "" {
		return nil, fmt.Errorf("cannot resolve default branch %q locally or as origin/%s", defaultBranch, defaultBranch)
	}

	mergeBase, err := exec.Command("git", "-C", repoRoot, "merge-base", baseRef, "HEAD").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot find merge base with %q: %w", baseRef, err)
	}
	mergeBaseHash := strings.TrimSpace(string(mergeBase))
	changed, err := exec.Command("git", "-C", repoRoot, "diff", "--name-only", "--diff-filter=ACMR", mergeBaseHash, "--", "*.sql").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list changed SQL files: %w", err)
	}
	untracked, err := exec.Command("git", "-C", repoRoot, "ls-files", "--others", "--exclude-standard", "--", "*.sql").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list untracked SQL files: %w", err)
	}

	unique := make(map[string]struct{})
	var files []string
	for _, line := range strings.Split(string(changed)+"\n"+string(untracked), "\n") {
		path := filepath.ToSlash(strings.TrimSpace(line))
		if path == "" {
			continue
		}
		for _, root := range []string{"migrations/", "db/migrations/", "sql/", "migration/"} {
			if strings.HasPrefix(path, root) {
				if _, exists := unique[path]; !exists {
					unique[path] = struct{}{}
					files = append(files, filepath.FromSlash(path))
				}
				break
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func resolveClassifier(offline bool) risk.RiskClassifier {
	if offline {
		return risk.NewHeuristicClassifier()
	}
	jevCfg := risk.LoadJevConfigFromEnv()
	if jevCfg.APIKey != "" {
		return risk.NewJevClassifier(jevCfg)
	}
	return risk.NewHeuristicClassifier()
}

func runRiskAnalyze(cwd, targetFile string, jsonOutput, githubOutput, offline bool) {
	fullPath := targetFile
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(cwd, targetFile)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading migration file %s: %v\n", targetFile, err)
		os.Exit(2)
	}

	cfg, _ := config.LoadConfig(cwd)
	engine := "postgres"
	if cfg != nil && cfg.Driver != "" {
		engine = cfg.Driver
	}

	schemaBranch := ""
	if branch, err := git.ResolveCurrentBranch(cwd); err == nil {
		schemaBranch = branch
	}
	introspector, closeIntrospector := resolveIntrospector(cfg, schemaBranch)
	defer closeIntrospector()
	analyzer, err := risk.NewRiskAnalyzer(risk.AnalyzeOptions{
		RepoRoot:     cwd,
		Engine:       engine,
		Offline:      offline,
		Classifier:   resolveClassifier(offline),
		Introspector: introspector,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize risk analyzer: %v\n", err)
		os.Exit(3)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	riskResult, err := analyzer.AnalyzeSQL(ctx, targetFile, string(content))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Risk analysis failed: %v\n", err)
		os.Exit(3)
	}

	report := analyzer.EvaluateReport([]risk.MigrationRisk{*riskResult}, "")
	_ = analyzer.SaveReport(report)

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		return
	}

	if githubOutput {
		fmt.Print(risk.FormatGitHubAnnotations(report))
		return
	}

	fmt.Print(risk.FormatPrettyTerminal(report))
}

func runRiskCheck(cwd, branchName, policyPath string, force, offline, jsonOutput bool) {
	if branchName == "" {
		resolved, err := git.ResolveCurrentBranch(cwd)
		if err == nil {
			branchName = resolved
		} else {
			branchName = "current"
		}
	}

	cfg, _ := config.LoadConfig(cwd)
	engine := "postgres"
	if cfg != nil && cfg.Driver != "" {
		engine = cfg.Driver
	}

	defaultBranch := "main"
	if cfg != nil && cfg.Proxy.DefaultBranch != "" {
		defaultBranch = cfg.Proxy.DefaultBranch
	}
	introspector, closeIntrospector := resolveIntrospector(cfg, branchName)
	defer closeIntrospector()
	analyzer, err := risk.NewRiskAnalyzer(risk.AnalyzeOptions{
		RepoRoot:     cwd,
		Engine:       engine,
		Offline:      offline,
		Force:        force,
		PolicyPath:   policyPath,
		Classifier:   resolveClassifier(offline),
		Introspector: introspector,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize risk analyzer: %v\n", err)
		os.Exit(2)
	}

	migrationFiles, err := changedMigrationFiles(cwd, defaultBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to identify pending migrations: %v\n", err)
		os.Exit(3)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var assessedRisks []risk.MigrationRisk
	for _, f := range migrationFiles {
		fullPath := filepath.Join(cwd, f)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read migration %s: %v\n", f, err)
			os.Exit(3)
		}

		res, err := analyzer.AnalyzeSQL(ctx, f, string(content))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to analyze migration %s: %v\n", f, err)
			os.Exit(3)
		}
		assessedRisks = append(assessedRisks, *res)
	}

	report := analyzer.EvaluateReport(assessedRisks, branchName)
	_ = analyzer.SaveReport(report)

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else if len(assessedRisks) > 0 {
		fmt.Print(risk.FormatPrettyTerminal(report))
	}

	if report.Blocked {
		if force {
			fmt.Println("⚠️  CRITICAL risk detected, but proceeding due to --force flag.")
			os.Exit(0)
		}
		fmt.Println("❌ Migration blocked by risk gate policy. Use 'branchbase risk analyze <file>' or pass --force.")
		os.Exit(1)
	}

	if report.RequiresConfirm && !force {
		confirmed, err := risk.PromptConfirmation(os.Stdin, os.Stdout, "High risk detected. Proceed with migration?")
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
			os.Exit(1)
		}
		if !confirmed {
			fmt.Println("Aborted by user.")
			os.Exit(1)
		}
	}

	// Exit 0: Proceed
	os.Exit(0)
}
