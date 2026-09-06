# BranchBase Roadmap 🗺️

This document outlines the milestones and engineering priorities for **BranchBase**.

---

## 🎯 Release Milestones

### 📍 Phase 0: Project Inception & RFC (Current)
- [x] Initial problem definition & community validation.
- [x] Architectural design specification (`ARCHITECTURE.md`).
- [x] Contributor guidelines & developer experience scaffolding.
- [ ] Open Call for Core Collaborators on GitHub Discussions / Twitter / Reddit / Hacker News.

### 📍 Phase 1: MVP Release (v0.1.0-alpha)
*Focus: End-to-end working loop for PostgreSQL + Git Hooks on macOS and Linux.*
- [ ] **CLI Core Framework:**
  - `branchbase init`: Detect Git repository and interactive database config.
  - `branchbase status`: Display active branch, attached database, and disk size.
  - `branchbase list`: List all managed branch databases.
  - `branchbase switch <branch>`: Manual switch fallback.
- [ ] **Git Hook Engine:**
  - Automated installation into `.git/hooks/post-checkout` and `.git/hooks/post-merge`.
  - Non-intrusive hook runner with execution timeout safeguards.
- [ ] **PostgreSQL Driver:**
  - Connection pooling and connection termination for `TEMPLATE` cloning.
  - Safe sanitization of Git branch names to valid PostgreSQL identifiers.
- [ ] **Transparent TCP Proxy (v1):**
  - PostgreSQL wire-protocol packet rewriter for `StartupMessage`.
  - Zero-latency bidirectional streaming.
- [ ] **Automated Testing Suite:**
  - End-to-end integration tests using Docker and testcontainers.

### 📍 Phase 2: SQLite & Multi-OS Hardening (v0.2.0)
*Focus: Single-file SQLite databases & cross-platform support.*
- [ ] **SQLite Driver:**
  - Copy-on-Write (CoW) implementation using `clonefile()` on macOS (APFS).
  - Linux `ioctl(FICLONE)` reflink support for Btrfs and XFS.
  - Windows fast stream-copy fallback.
  - WAL checkpoint flushing prior to snapshotting.
- [ ] **Cleanup & Pruning:**
  - `branchbase prune`: Detect merged and orphaned Git branches and delete corresponding databases.
  - Interactive deletion confirmation (`--dry-run` and `--force`).
- [ ] **Terminal UI (TUI):**
  - Interactive terminal dashboard to view branches, database sizes, and active queries.

### 📍 Phase 3: Ecosystem & Advanced Workflows (v0.3.0)
*Focus: MySQL, Docker Compose deep integration, and multi-agent workflows.*
- [ ] **MySQL / MariaDB Driver:**
  - Schema recreation and row streaming via `mysqldump` / volume cloning.
- [ ] **Docker Compose Native Integration:**
  - Auto-detection of `services.*.ports` and `environment` in `docker-compose.yml`.
  - One-click setup for existing Dockerized teams.
- [ ] **Multi-Agent / Worktree Support:**
  - Native support for `git worktree` so AI coding agents and human developers can run in parallel without port or DB collision.

---

## 🏷️ Looking for Tasks to Hack On?

Check our GitHub Issues tagged with:
- `good first issue` — Beginner-friendly tasks to get started.
- `help wanted` — Key features requiring design and community input.
- `driver` — New database engine implementations.
