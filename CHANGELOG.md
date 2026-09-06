# Changelog

All notable changes to **BranchBase** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
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
