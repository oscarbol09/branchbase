package doctor

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/driver"
	"github.com/branchbase/branchbase/internal/git"
	"github.com/branchbase/branchbase/internal/hook"
)

// CheckResult represents the outcome of an individual health check.
type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// DoctorReport contains all check results and an overall status flag.
type DoctorReport struct {
	Checks    []CheckResult `json:"checks"`
	AllPassed bool          `json:"all_passed"`
}

func driverParamsForConfig(cfg *config.Config) map[string]interface{} {
	params := make(map[string]interface{})
	params["host"] = cfg.Connection.Host
	params["port"] = cfg.Connection.Port
	params["user"] = cfg.Connection.User
	params["password"] = cfg.Connection.Password
	params["base_database"] = cfg.Connection.BaseDatabase
	params["sslmode"] = "disable"
	if strings.TrimSpace(cfg.Proxy.DefaultBranch) != "" {
		params["default_branch"] = cfg.Proxy.DefaultBranch
	}
	params["max_open_conns"] = 1
	params["max_idle_conns"] = 1

	if cfg.Connection.Path != "" {
		params["path"] = cfg.Connection.Path
	} else if cfg.Connection.BaseDatabase != "" {
		params["path"] = cfg.Connection.BaseDatabase
	}
	return params
}

// Run performs all environment diagnostic checks.
func Run(ctx context.Context, repoPath, customConfig string) *DoctorReport {
	report := &DoctorReport{
		Checks:    make([]CheckResult, 0, 6),
		AllPassed: true,
	}

	addCheck := func(res CheckResult) {
		if !res.Passed {
			report.AllPassed = false
		}
		report.Checks = append(report.Checks, res)
	}

	// 1. Git Repository Check
	branch, err := git.ResolveCurrentBranch(repoPath)
	if err != nil {
		addCheck(CheckResult{
			Name:    "Git Repository",
			Passed:  false,
			Message: "invalid or missing .git directory",
			Details: err.Error(),
		})
	} else {
		addCheck(CheckResult{
			Name:    "Git Repository",
			Passed:  true,
			Message: fmt.Sprintf("valid (active branch: '%s')", branch),
		})
	}

	// 2. Git Hooks Check
	if !hook.AreHooksInstalled(repoPath) {
		addCheck(CheckResult{
			Name:    "Git Hooks",
			Passed:  false,
			Message: "hooks not installed (run 'branchbase hooks install')",
		})
	} else {
		addCheck(CheckResult{
			Name:    "Git Hooks",
			Passed:  true,
			Message: "active in .git/hooks/post-checkout",
		})
	}

	// 3. Configuration Check
	targetConfig := repoPath
	if customConfig != "" {
		targetConfig = customConfig
	}
	cfg, loadErr := config.LoadConfig(targetConfig)

	if loadErr != nil {
		addCheck(CheckResult{
			Name:    "Config",
			Passed:  false,
			Message: "failed to load configuration file",
			Details: loadErr.Error(),
		})
	} else {
		configFileName := ".branchbase.json"
		if customConfig != "" {
			configFileName = filepath.Base(customConfig)
		}
		addCheck(CheckResult{
			Name:    "Config",
			Passed:  true,
			Message: fmt.Sprintf("%s loaded successfully", configFileName),
		})
	}

	if cfg == nil {
		defaultCfg := config.DefaultConfig()
		cfg = &defaultCfg
	}

	// 4. Database Backend Connectivity
	drvName := cfg.Driver
	if drvName == "" {
		drvName = "postgres"
	}
	drv, err := driver.GetDriver(drvName, driverParamsForConfig(cfg))
	if err != nil {
		addCheck(CheckResult{
			Name:    "Database Backend",
			Passed:  false,
			Message: fmt.Sprintf("failed to initialize %s driver", drvName),
			Details: err.Error(),
		})
	} else {
		defer func() {
			if err := drv.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "doctor: failed to close driver: %v\n", err)
			}
		}()

		pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
		defer pingCancel()

		if err := drv.Ping(pingCtx); err != nil {
			backendAddr := fmt.Sprintf("%s:%d", cfg.Connection.Host, cfg.Connection.Port)
			if cfg.Driver == "sqlite" {
				backendAddr = cfg.Connection.Path
				if backendAddr == "" {
					backendAddr = cfg.Connection.BaseDatabase
				}
			}
			addCheck(CheckResult{
				Name:    "Database Backend",
				Passed:  false,
				Message: fmt.Sprintf("%s on %s (unreachable)", strings.ToUpper(cfg.Driver), backendAddr),
				Details: err.Error(),
			})
		} else {
			backendAddr := fmt.Sprintf("%s:%d", cfg.Connection.Host, cfg.Connection.Port)
			if cfg.Driver == "sqlite" {
				backendAddr = cfg.Connection.Path
				if backendAddr == "" {
					backendAddr = cfg.Connection.BaseDatabase
				}
			}
			addCheck(CheckResult{
				Name:    "Database Backend",
				Passed:  true,
				Message: fmt.Sprintf("%s on %s (reachable)", strings.ToUpper(cfg.Driver), backendAddr),
			})

			// 5. Base Database Check
			defaultBranch := cfg.Proxy.DefaultBranch
			if defaultBranch == "" {
				defaultBranch = "main"
			}
			sanitizedBase := git.SanitizeBranchName(defaultBranch)
			existsCtx, existsCancel := context.WithTimeout(ctx, 3*time.Second)
			defer existsCancel()

			exists, err := drv.BranchExists(existsCtx, sanitizedBase)
			baseDbName := cfg.DatabaseNameForBranch(sanitizedBase)
			if err != nil || !exists {
				addCheck(CheckResult{
					Name:    "Base Database",
					Passed:  false,
					Message: fmt.Sprintf("'%s' does not exist or cannot be verified", baseDbName),
				})
			} else {
				addCheck(CheckResult{
					Name:    "Base Database",
					Passed:  true,
					Message: fmt.Sprintf("'%s' exists", baseDbName),
				})
			}
		}
	}

	// 6. Proxy Port Check
	listenPort := cfg.Proxy.ListenPort
	if listenPort <= 0 {
		listenPort = 5432
	}
	addr := fmt.Sprintf("127.0.0.1:%d", listenPort)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		addCheck(CheckResult{
			Name:    "Proxy Port",
			Passed:  false,
			Message: fmt.Sprintf("%d is currently in use or unavailable", listenPort),
			Details: err.Error(),
		})
	} else {
		_ = l.Close()
		addCheck(CheckResult{
			Name:    "Proxy Port",
			Passed:  true,
			Message: fmt.Sprintf("%d is available", listenPort),
		})
	}

	return report
}

// FormatReport outputs the doctor report in standard CLI formatting.
func FormatReport(report *DoctorReport) string {
	var sb strings.Builder
	sb.WriteString("🌿 BranchBase Doctor\n")

	for _, check := range report.Checks {
		icon := "  [✔]"
		if !check.Passed {
			icon = "  [✖]"
		}
		sb.WriteString(fmt.Sprintf("%s %s: %s\n", icon, check.Name, check.Message))
		if !check.Passed && check.Details != "" {
			sb.WriteString(fmt.Sprintf("      ↳ %s\n", check.Details))
		}
	}

	if report.AllPassed {
		sb.WriteString("All checks passed! Your environment is ready.\n")
	} else {
		sb.WriteString("Some checks failed. Please address the issues above.\n")
	}

	return sb.String()
}
