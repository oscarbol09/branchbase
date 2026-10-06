# Changelog

All notable changes to **BranchBase** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- **Native MongoDB Database Driver (`internal/driver/mongodb`)**: Full collection and metadata cloning using the Aggregation Framework (`$out` pipeline) and automated index reconstruction (#94).
- **Environment Diagnostic Command (`branchbase doctor`)**: Comprehensive health check diagnosing Git repository integrity, hook installation, configuration validity, database connectivity, base database presence, and proxy port availability (#92).
- **MySQL / MariaDB Wire-Protocol Interceptor (`internal/proxy/mysqlwire`)**: Inverted handshake handler intercepting `Protocol::HandshakeV10`, parsing `HandshakeResponse41`, and rewriting target database schemas for transparent MySQL connection routing (#87).
- **MySQL Views, Triggers, and Generated Columns Replication**: Schema-level character set and collation inheritance, column filtering to prevent Error 3105 on virtual/stored generated columns, and automatic view and trigger DDL replication during branch creation (#112).
- **Live Multi-Database E2E Test Suite**: Expanded GitHub Actions CI workflow with live containerized PostgreSQL 15, MySQL 8, and MongoDB 7 services.
- **BranchBase Risk Gate (`branchbase risk`)**: Automated schema migration risk classification engine safeguarding preview and branch databases from destructive DDL (#95).
- **TypeSafe Jev System One Integration (`internal/risk/jev.go`)**: Multi-question typed classification with calibrated probabilities for data loss, foreign key impact, and table locking.
- **Deterministic Heuristic Offline Classifier (`internal/risk/heuristic.go`)**: Offline rule engine for recognized PostgreSQL, MySQL, and SQLite migration statements; unclassified SQL is escalated for manual review.
- **Composite Analysis Cache (`internal/risk/cache.go`)**: Local `SHA256(migration_sql + schema_signature)` cache in `.branchbase/cache/risk_cache.json` avoids repeating analysis for matching inputs.
- **Database Catalog Introspection (`internal/risk/introspect.go`)**: Live PostgreSQL `information_schema` foreign key constraint inspector.
- **Policy Engine as Code (`.branchbase/risk-policy.yml`)**: Declarative configuration for risk thresholds, actions (`allow`, `warn`, `confirm`, `block`), and exclusion globs.
- **TTY Detection & Safe Prompts (`internal/risk/tty.go`)**: Non-blocking fail-safe confirmation in non-interactive terminal contexts (VS Code Git, GUI clients).

### Fixed
- **Proxy TLS Connection Descriptor Leak**: Explicitly tracked and cleaned up raw socket descriptors alongside TLS wrappers in deferred execution blocks, preventing memory leaks during SSL sessions (#108).
- **TUI Raw Terminal Mode & ANSI Lookahead**: Integrated `golang.org/x/term` with `term.MakeRaw` and buffered lookahead parsing for ANSI escape sequences to prevent accidental exits on arrow key navigation (#110).
- **SQLite WAL Checkpoint Integrity**: Enforced `PRAGMA wal_checkpoint(TRUNCATE)` before snapshotting to ensure consistent database copies (#113).
- **PostgreSQL Connection Termination Before Cloning**: Terminated lingering connections to template databases in `CreateBranch` to prevent cloning errors.
- Compare `DatabaseNameForBranch` against a sanitized `DefaultBranch` so names like `release/v1` resolve to the base database instead of `base_release_v1` (#91).

## [0.3.0] - 2026-09-16

### Highlights
- **Interactive Terminal UI (TUI) Dashboard (`branchbase tui`)**: Pure-Go zero-dependency dashboard with real-time database inspection and keyboard switching (#74).
- **Multi-Engine Driver Architecture**: Support for PostgreSQL 14+, MySQL 8.0+, MariaDB, and SQLite with native APFS `clonefile(2)` CoW snapshots on macOS (#75, #66).
- **Enterprise Proxy Hardening**: Full PostgreSQL wire protocol rewriting, TLS negotiation (`sslmode=require`), UNIX domain sockets (`/tmp/.s.PGSQL.5432`), and graceful active connection draining (#50, #49, #34).
- **CI/CD Quality Gates & Live PostgreSQL Integration**: Dedicated E2E integration test suite in GitHub Actions running with live PostgreSQL 16 service container (#51).
- **Git Hooks & CLI Robustness**: Protection against `SIGHUP` using `nohup`, `--skip-hooks` support in `branchbase init`, and automatic Docker Compose detection (#65, #45, #75).

### Security
- Standard SQL identifier quoting via `pq.QuoteIdentifier` in `CreateBranch` and `DeleteBranch`, preventing quoting discrepancies in PostgreSQL (#71).
- Sane wildcard escaping (`escapeLikeWildcards`) with `ESCAPE '\'` in PostgreSQL `ListBranches` query, eliminating false-positive matches for databases with underscores (#71).
- Strict CI/CD security quality gate enforcement in `govulncheck` by removing `continue-on-error` (#71).
- Strict target database identifier validation and sanitization in `pgwire.RewriteDatabase()`, enforcing standard PostgreSQL naming conventions, a 63-byte limit, and rejecting embedded null bytes (#41).

### Added
- Native MySQL and MariaDB database driver (`internal/driver/mysql`) with schema cloning, table replication, backtick identifier quoting, and `information_schema` storage metrics (#75).
- Docker Compose auto-detection engine (`internal/compose`) parsing `docker-compose.yml` and `compose.yaml` to extract database services, ports, and environment credentials during `branchbase init` (#75).
- Interactive Terminal UI (TUI) dashboard (`branchbase tui` / `branchbase dashboard`) with zero external dependencies, ANSI escape sequences, arrow/vim keyboard navigation, branch switching, and live database inspection (#74).
- PostgreSQL wire protocol `ErrorResponse` (`'E'`) packet generator (`pgwire.BuildErrorResponse`) providing detailed client-side diagnostics (Severity, SQLSTATE, message) upon JIT provisioning or routing failures (#71).
- Transparent Git worktree and submodule support in hook installer (`internal/hook/hook.go`) resolving pointer files (`gitdir: ...`) and `commondir` (#71).

### Performance
- Single-file checkout event filtering in `branchbase hook-trigger` (`flag == "0"` from Git `post-checkout`), skipping redundant database driver connections and queries when not switching branches (#71).

### Added
- Native Apple File System (APFS) `clonefile(2)` syscall in macOS SQLite driver (`internal/driver/sqlite/clone_darwin.go`) with chunked copy fallback (#66).
- Graceful active connection draining with timeout and forced termination on proxy shutdown in `internal/proxy` (#49).
- UNIX domain socket support for proxy listener and database backend targets (`/tmp/.s.PGSQL.5432`) (#50).
- TLS/SSL client negotiation and self-signed development certificates for `sslmode=require` connections (#34).
- Database metadata tracking and `ErrBranchNameCollision` guard in database drivers preventing cross-branch name collisions (#60).
- End-to-End integration test suite in GitHub Actions running with PostgreSQL 16 service container (`//go:build integration`) (#51).
- Complete Prisma ORM developer integration guide (`docs/guides/prisma.md`) (#21).

### Fixed
- Transparent forwarding of PostgreSQL `CancelRequest` (`1234.5678`) wire packets in proxy without startup rewriting or protocol corruption (#46).
- Strict pre-validation of PostgreSQL 63-byte identifier limit in `PostgresDriver.CreateBranch()` preventing silent database truncation (#63).
- Pinned official stable GitHub Actions in CI/CD pipeline (`actions/checkout@v4`, `actions/setup-go@v5`, `golangci/golangci-lint-action@v6`) (#71).


### Fixed
- PostgreSQL driver connection pool initialization with `lib/pq` and `database/sql`, supporting SQLSTATE `42P04` (`duplicate_database`) idempotent branching and hermetic `sqlmock` tests (#24).
- Explicit `Close() error` resource teardown across database drivers and CLI proxy lifecycle (#24).
- Thread-safe `Server.listener` access synchronized with a mutex across `Start()`, `acceptLoop()`, and `Stop()`, eliminating data races under `-race` (#39).
- Idempotent and thread-safe `Server.Stop()` shutdown via `sync.Once`, preventing channel close panics on repeated calls (#35).
- Accurate `.branchbase.yaml` configuration parsing using `yaml.v3`, preserving defaults for unspecified fields (#36).
- Explicit `ErrPacketTooLarge` error returned when client startup packets exceed the 10KB limit (#37).

### Added
- Proxy Just-in-Time (JIT) branch database provisioning (`internal/proxy`) under double-checked locking with refcounted `keyedMutex` (#31).
- Environment variable expansion (`${ENV_VAR}` and `$ENV_VAR`) in configuration files with `.branchbase.yml` extension support (#48).
- Git branch discovery (`ResolveLocalBranches`) and merged branch resolution (`ResolveMergedBranches`) with packed-refs and fallback inspection (#48).
- Dynamic branch provisioning on `branchbase switch <branch>` with `--no-create` flag (#29).
- Formatted ASCII table and structured `--json` output for `branchbase list` (#27).
- Merged and orphaned branch database reconciliation in `branchbase prune` with `--dry-run`, interactive `[y/N]` confirmation, and `--force` flag (#28, #53).
- Non-intrusive background branch database pre-warming in `branchbase hook-trigger` with 5-second context timeout and `.branchbase.log` error tracking (#30).
- Hermetic end-to-end CLI integration tests (`cmd/branchbase/main_test.go`) utilizing pure-filesystem SQLite driver in `t.TempDir()`.
- Pure Go SQLite database driver (`internal/driver/sqlite`) supporting filesystem Copy-on-Write snapshots (Linux ioctl `FICLONE`, macOS, Windows streaming fallback) and WAL/SHM replication (#2).
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
