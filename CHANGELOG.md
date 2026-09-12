# Changelog

All notable changes to **BranchBase** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- Native Go fuzz testing (`testing.F`) for `pgwire.ParseStartupMessage` and `git.SanitizeBranchName`.
- Automated vulnerability scanning in CI via `govulncheck`.
- Concurrency cancellation (`cancel-in-progress`) and 10-minute job timeouts in CI/CD pipeline.
- Race detector (`-race`) execution on Unix runners in CI matrix.
- Proxy server lifecycle and graceful shutdown tests (`internal/proxy/proxy_test.go`).
- Database driver safety guard tests preventing deletion of protected base databases.
- Comprehensive negative and boundary tests for Git HEAD resolver, worktrees, and detached states.
- `--json` output flag for `branchbase status` providing structured JSON output for scripts and devtools (#17).
- Bidirectional socket cleanup and channel draining in proxy `handleConnection`, eliminating goroutine and file descriptor leaks on half-close (#18).
- Defensive bounds checks and error reporting for inverted or corrupted hook markers in `UninstallHooks` (#15).
- Nil-safe fallback in `runStatus` when `.branchbase.json` is missing or nil (#16).
- CI matrix compatibility fix for macOS ARM64 runners requiring Go 1.23+ dyld `LC_UUID` (#20).
- PostgreSQL wire protocol `StartupMessage` packet parser and dynamic rewriter (`internal/proxy/pgwire`).
- Automated Git hook engine (`internal/hook`) installing `post-checkout` and `post-merge` hooks.
- CLI subcommands for hook lifecycle management: `branchbase hooks [install|uninstall|status]`.
- Non-intrusive background hook trigger with 5-second timeout safeguard (`branchbase hook-trigger`).
- Manual branch switching command (`branchbase switch <branch>`).
- Unit test suites for `pgwire`, `hook`, `config`, and `postgres` driver.
- Community RFC discussion #6 for Phase 0 open collaboration call.
- Transparent TCP proxy server with dynamic Git branch routing.
- PostgreSQL driver skeleton utilizing `CREATE DATABASE ... TEMPLATE`.
- Git HEAD branch resolver and SQL identifier sanitization engine.
- Configuration manager supporting `.branchbase.json` and `.branchbase.yaml`.
- CLI commands: `init`, `status`, `proxy`, `list`, `prune`, `version`.
- Architecture specification (`ARCHITECTURE.md`) with Mermaid sequence diagrams.
- Contributing guidelines (`CONTRIBUTING.md`), Code of Conduct (`CODE_OF_CONDUCT.md`), and Security Policy (`SECURITY.md`).
- Multi-OS GitHub Actions CI workflow (Ubuntu, macOS, Windows).
- Pre-commit configuration and EditorConfig standardization.
- BranchBase driver skill (`.agents/skills/branchbase-driver/SKILL.md`).

---

## [0.1.0-alpha] - 2026-09-06

### Inception
- Initial project scaffolding and public repository launch.
