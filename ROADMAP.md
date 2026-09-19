# BranchBase Roadmap

This document outlines the milestones and engineering priorities for **BranchBase**.

---

## Release Milestones

### Phase 0: Project Inception & RFC (Completed)
- [x] Initial problem definition & community validation.
- [x] Architectural design specification (`ARCHITECTURE.md`).
- [x] Contributor guidelines & developer experience scaffolding.
- [x] Open Call for Core Collaborators on GitHub Discussions (#6).

### Phase 1: MVP Release (v0.1.0-alpha - Completed)
*Focus: End-to-end working loop for PostgreSQL + Git Hooks on macOS, Linux, and Windows.*
- [x] **CLI Core Framework:**
  - `branchbase init`: Detect Git repository, create `.branchbase.json`, and install hooks.
  - `branchbase status`: Display active branch, attached database, proxy port, and hook status.
  - `branchbase list`: List all managed branch databases.
  - `branchbase switch <branch>`: Manual switch fallback.
- [x] **Git Hook Engine:**
  - Automated installation into `.git/hooks/post-checkout` and `.git/hooks/post-merge`.
  - Non-intrusive hook runner with 5-second execution timeout safeguards.
  - `branchbase hooks [install|uninstall|status]` CLI management.
- [x] **PostgreSQL Driver:**
  - Connection pooling and connection termination for `TEMPLATE` cloning.
  - Safe sanitization of Git branch names to valid PostgreSQL identifiers.
- [x] **Transparent TCP Proxy (v1):**
  - PostgreSQL wire-protocol packet rewriter for `StartupMessage` (`internal/proxy/pgwire`).
  - SSL negotiation handler ('N' decline for local inspection).
  - Zero-latency bidirectional streaming.
- [x] **Automated Testing Suite:**
  - Multi-OS GitHub Actions CI workflow (Ubuntu, macOS, Windows).
  - Unit test suite for resolver, config, pgwire, hooks, and drivers.

### Phase 2: SQLite & Multi-OS Hardening (v0.2.0 - Completed)
*Focus: Single-file SQLite databases, interactive terminal dashboard & cross-platform support.*
- [x] **SQLite Driver:**
  - Copy-on-Write (CoW) implementation on macOS.
  - Linux `ioctl(FICLONE)` reflink support for Btrfs and XFS.
  - Windows fast stream-copy fallback.
  - WAL and SHM sidecar file replication and crash-recovery support.
- [x] **Cleanup & Pruning:**
  - `branchbase prune`: Detect merged and orphaned Git branches and delete corresponding databases.
  - Interactive deletion confirmation (`--dry-run` and `--force`).
- [x] **Terminal UI (TUI):**
  - Interactive terminal dashboard (`branchbase tui` / `branchbase dashboard`) with keyboard navigation (`↑`/`↓`, `j`/`k`), branch switching (`Enter`/`s`), and real-time refresh (`r`).

### Phase 3: Ecosystem & Advanced Workflows (v0.3.0 - Completed)
*Focus: MySQL, Docker Compose deep integration, proxy hardening, and multi-agent workflows.*
- [x] **MySQL / MariaDB Driver:**
  - Native pure-Go driver (`internal/driver/mysql`) for MySQL 8+ and MariaDB.
  - Dynamic schema creation (`CREATE DATABASE`) and table/row replication (`CREATE TABLE ... LIKE`, `INSERT INTO ... SELECT`).
  - Safe backtick identifier quoting and storage tracking via `information_schema.TABLES`.
- [x] **Docker Compose Native Integration:**
  - Auto-detection of `docker-compose.yml`, `docker-compose.yaml`, `compose.yml`, `compose.yaml` (`internal/compose`).
  - Extraction of database services (PostgreSQL, MySQL, MariaDB), published port bindings, environment variables (`POSTGRES_DB`, `MYSQL_DATABASE`, etc.).
  - Zero-config auto-population of `.branchbase.json` during `branchbase init`.
- [x] **Enterprise Proxy Hardening:**
  - TLS/SSL client negotiation (`sslmode=require`) with self-signed ECDSA development certificates (#34).
  - UNIX domain socket support for proxy listener and backend targets (`/tmp/.s.PGSQL.5432`) (#50).
  - Graceful active connection draining with configurable timeout on proxy shutdown (#49).
- [x] **macOS APFS Native Cloning:**
  - `clonefile(2)` syscall for instant Copy-on-Write SQLite snapshots on APFS, with universal chunked copy fallback (#66).
- [x] **CI/CD Quality Gates:**
  - End-to-End integration test suite in GitHub Actions with live PostgreSQL 16 service container (#51).
  - 8-job CI matrix: Ubuntu, macOS, Windows, Lint, Security Audit, E2E Integration.
- [x] **Git Hook & CLI Robustness:**
  - `nohup` protection against `SIGHUP` in hook script template (#65).
  - `--skip-hooks` / `--no-hooks` flag in `branchbase init` (#45).
  - Branch metadata tracking and `ErrBranchNameCollision` guard in database drivers (#60).
- [x] **Prisma ORM Integration Guide:**
  - Complete developer guide for using BranchBase with Prisma ORM (`docs/guides/prisma.md`) (#21).
- [x] **Multi-Agent / Parallel Worktree Isolation:**
  - [x] Native Git worktree detection (`ResolveCurrentBranch`) and hook lifecycle (`InstallHooks`, `UninstallHooks`, `AreHooksInstalled`) across linked worktrees.
  - [x] Non-conflicting proxy port resolution and connection pooling across isolated worktree instances.

---

## Open Tasks & Good First Issues

Check our GitHub Issues tagged with:
- `good first issue` — Beginner-friendly tasks to get started.
- `help wanted` — Key features requiring design and community input.
- `driver` — New database engine implementations.
