# BranchBase

> **Git-native local database branching and wire-protocol routing for local development.**  
> Isolates schema changes and migrations per Git branch without re-seeding or editing `.env`.

[![BranchBase CI](https://github.com/oscarbol09/branchbase/actions/workflows/ci.yml/badge.svg)](https://github.com/oscarbol09/branchbase/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Good First Issues](https://img.shields.io/github/issues/oscarbol09/branchbase/good%20first%20issue?color=7057ff&label=good%20first%20issues)](https://github.com/oscarbol09/branchbase/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
[![GitHub Sponsors](https://img.shields.io/badge/Sponsor-GitHub-ea4aaa?logo=github-sponsors&logoColor=white)](https://github.com/sponsors/oscarbol09)
[![Support on Ko-Fi](https://img.shields.io/badge/Support-Ko--Fi-F16061?logo=ko-fi&logoColor=white)](https://ko-fi.com/oscarmb09)

---

## Problem Context: Git vs. Local Database Friction

Every developer working with Docker, local PostgreSQL, MySQL, SQLite, or MongoDB has suffered this loop:

```text
1. You work on `feature/checkout-v2`.
   └── Ran migrations: added table `stripe_orders`, added column `users.billing_tier NOT NULL`.
2. Urgent production bug alert! You run:
   └── `git checkout main`
3. You start the app or run tests on `main`.
4. 💥 CRASH:
   └── ActiveRecord::PendingMigrationError / PrismaClientKnownRequestError:
       "column users.billing_tier does not exist" or "schema mismatch detected".
```

### How developers waste hours today:
* **The Nuclear Option:** `docker compose down -v && docker compose up -d`  
  *(Loses all your test seeds, logins, and mocked state. Takes minutes to re-seed).*
* **The Manual Rollback Dance:** Trying to rollback migrations on `feature/checkout-v2` before switching, only to lose experimental test data.
* **The `.env` Nightmare:** Manually maintaining `DATABASE_URL_DEV`, `DATABASE_URL_CHECKOUT`, and editing `.env` on every branch change.
* **Cloud Branching (Neon, PlanetScale):** Great developer experience, but **proprietary, paid, and requires an internet connection**. It doesn't work for offline development or standard local Docker setups.

---

## The Solution: BranchBase

BranchBase pairs automated Git branch transitions with database cloning mechanisms and a transparent wire-protocol TCP proxy, keeping your connection strings stable while routing queries to branch-specific databases.

```
                           +---------------------------+
                           |     Developer Machine     |
                           +---------------------------+
                                         |
                                `git checkout branch-b`
                                         |
                                         v
                            [ BranchBase Git Hook ]
                                         |
                    +--------------------+--------------------+
                    |                                         |
          (Detects new branch)                      (Branch Provisioning)
                    |                                         |
                    v                                         v
+---------------------------------------+   +------------------------------------+
|   BranchBase Proxy (Postgres / MySQL) |   |          Local Database            |
+---------------------------------------+   +------------------------------------+
| - App connection string stays stable  |   | - `db_project_main` (frozen)       |
| - Automatically routes queries to the |   | - `db_project_branch_b` (active)   |
|   active Git branch database!         |   |   (Created via engine driver)      |
+---------------------------------------+   +------------------------------------+
```

### Key Highlights
- **Engine-Native Branching:** Creates fresh, isolated branch databases using PostgreSQL `CREATE DATABASE ... TEMPLATE`, MySQL schema/data cloning, SQLite filesystem Copy-on-Write (`clonefile` / `FICLONE`), or MongoDB aggregation cloning.
- **Transparent Wire Proxy:** Your application connects to a stable local port (`5432` for PostgreSQL or `3306` for MySQL). The proxy inspects the active Git branch from `.git/HEAD` and routes connections to that branch's database without touching `.env`.
- **Automated Git Lifecycle:** Hooks into `post-checkout` and `post-merge` via lightweight POSIX scripts that run asynchronously.
- **Automatic Reconcile & Cleanup (`prune`):** Detects merged or deleted branches and safely removes orphaned databases.
- **100% Local & Offline:** Operates strictly on localhost with zero cloud telemetry or proprietary subscriptions.

---

## Command & CLI Reference

| Command | What it does |
| :--- | :--- |
| `branchbase init [--skip-hooks]` | Interactively inspect repository and generate `.branchbase.json` (optionally skip hook installation) |
| `branchbase proxy` | Start the local transparent TCP routing proxy (default port: `5432`) with JIT provisioning |
| `branchbase status [--json]` | Display active Git branch, sanitized name, target DB, and proxy status |
| `branchbase list [--json]` | List all active and ephemeral databases managed by BranchBase with size and status |
| `branchbase switch <branch> [--no-create]` | Manually switch or provision an isolated database for a specific branch |
| `branchbase tui` / `dashboard` / `ui` | Launch interactive terminal UI dashboard with keyboard navigation and branch switching |
| `branchbase hooks install` | Install automated `post-checkout` and `post-merge` hooks into `.git/hooks/` |
| `branchbase hooks uninstall` | Remove BranchBase hooks from `.git/hooks/` |
| `branchbase hooks status` | Inspect Git hooks installation and activity status |
| `branchbase doctor` | Diagnose system prerequisites, Docker health, dependencies, and port availability |
| `branchbase risk analyze <file> [--json] [--offline] [--github]` | Parse and evaluate migration file risk |
| `branchbase risk check [--branch <b>] [--policy <p>] [--force] [--offline] [--json]` | Evaluate pending branch migrations against policy |
| `branchbase completion <bash|zsh|fish|powershell>` | Generate tab-completion scripts |
| `branchbase prune [--dry-run] [--force]` | Reconcile merged/orphaned branches and safely delete corresponding databases |
| `branchbase version` | Print the current BranchBase version, author, and repository URL |

Risk policy `exclude_paths` patterns ending in `/**` exclude that directory and
its descendants. For example, `migrations/seeds/**` excludes
`migrations/seeds/users/001.sql`, but not `migrations/seeds_archive/001.sql` or
`migrations/seeds.sql`; those paths still undergo risk assessment.

---

## Repository Structure

```text
branchbase/
├── cmd/
│   └── branchbase/
│       └── main.go               # CLI entry point (subcommands & signal handling)
├── internal/
│   ├── compose/                  # Docker Compose auto-detection & environment parser
│   ├── config/                   # Configuration loader (.branchbase.json / .yaml)
│   ├── doctor/                   # System health, port & dependency diagnostics
│   ├── driver/                   # Database engine interfaces & registry
│   │   ├── driver.go             # Core Driver interface contract
│   │   ├── mysql/                # MySQL & MariaDB engine (table cloning & metadata)
│   │   ├── postgres/             # PostgreSQL engine (TEMPLATE cloning)
│   │   ├── mongodb/              # MongoDB engine (aggregation cloning)
│   │   └── sqlite/               # SQLite engine (CoW / Reflink snapshots)
│   ├── git/                      # Git HEAD inspector and branch sanitization
│   │   ├── resolver.go           # Non-subshell .git/HEAD resolution
│   │   └── resolver_test.go      # Table-driven unit test suite
│   ├── hook/                     # Automated Git hook manager (post-checkout/merge)
│   │   ├── hook.go               # Non-intrusive hook installer
│   │   └── hook_test.go          # Hook lifecycle test suite
│   ├── proxy/                    # Transparent TCP proxy & wire routing
│   │   ├── mysqlwire/            # MySQL wire-protocol handshake parser & replacer
│   │   ├── pgwire/               # PostgreSQL wire-protocol StartupMessage rewriter
│   │   │   ├── pgwire.go         # Packet parser & database replacer
│   │   │   └── pgwire_test.go    # Protocol unit test suite
│   │   └── proxy.go              # Zero-overhead bidirectional TCP forwarder
│   ├── risk/                     # Migration Risk Gate engine (JEV + Heuristic classifier)
│   └── tui/                      # Interactive Terminal UI (ANSI dashboard)
├── .agents/                      # Custom Agent skills & development workflows
├── .github/                      # CI workflows, issue templates, dependabot
├── ARCHITECTURE.md               # Detailed system design & sequence diagrams
├── CONTRIBUTING.md               # Contributor guide & driver creation tutorial
├── SETUP.md                      # Local developer environment setup guide
├── SECURITY.md                   # Security policy & private vulnerability reporting
├── CODE_OF_CONDUCT.md            # Contributor Covenant v2.1
├── CHANGELOG.md                  # Keep a Changelog version history
├── branchbase.example.yaml       # Annotated configuration specification
└── go.mod                        # Go 1.26+ module definition
```

---

## How It Works

1. **Detection:** When you run `git checkout <branch>`, BranchBase's hook (`.git/hooks/post-checkout`) detects the branch transition in under 5ms by reading `.git/HEAD`.
2. **Identifier Sanitization:** Special characters like `/` or `-` in branch names (e.g. `feature/stripe-v2`) are converted into safe database identifiers (`feature_stripe_v2`).
3. **Copy-on-Write Snapshot:**
   * **PostgreSQL:** Disconnects lingering connections to the template and executes `CREATE DATABASE <target> TEMPLATE <source>;` (instant CoW clone).
   * **MySQL / MariaDB:** Copies schemas and rows table by table (`CREATE TABLE ... LIKE`, `INSERT INTO ... SELECT`); concurrent writes to the source are not captured from a single transaction snapshot.
   * **MongoDB:** Uses the $out aggregation pipeline to clone collections and reconstructs indexes.
   * **SQLite:** Performs a filesystem reflink/clone (`clonefile()` or `FICLONE`) only when the source has no active WAL/SHM/journal sidecars; otherwise it refuses the snapshot until clients stop and the database is checkpointed.
   * **Docker Compose:** Automatically inspects `docker-compose.yml` to configure database ports and credentials without manual input.
4. **Transparent Routing:** When your backend app queries `localhost:5432`, the BranchBase proxy intercepts the connection, resolves the active branch database, and forwards traffic directly with sub-millisecond overhead.
5. **Lifecycle Pruning:** Once a PR is merged into `main`, running `branchbase prune` removes the ephemeral database, freeing disk space.

---

## Known Limitations & Trade-offs

- **Proxy Protocol Scope:** The transparent TCP proxy routes PostgreSQL (`5432`) and MySQL (`3306`) wire protocols. File-based databases like SQLite and document engines like MongoDB are managed via CLI commands and Git hooks, but are not intercepted by the TCP proxy.
- **MySQL Wire SSL:** When the transparent MySQL proxy runs in cleartext mode, incoming client connections requesting TLS (`ClientSSL`) fail closed with MySQL error code `2061`. Applications using MySQL through the proxy must configure SSL as disabled or optional in local development.
- **PostgreSQL Connection Locking:** PostgreSQL's `CREATE DATABASE ... TEMPLATE` fails with SQLSTATE `55006` if active connections remain open on the template. BranchBase terminates idle connections via `pg_terminate_backend()` prior to cloning; long-running active transactions on the source database can still cause cloning to wait or fail.
- **MySQL Data Cloning Performance:** MySQL lacks native copy-on-write database cloning. BranchBase replicates schemas and data table by table (`CREATE TABLE ... LIKE`, `INSERT INTO ... SELECT`). Large seed datasets (e.g. hundreds of megabytes or millions of rows) will take seconds to clone rather than milliseconds.
- **SQLite Filesystem Reflink:** Fast Copy-on-Write snapshots depend on filesystem support (`clonefile` on APFS, `FICLONE` on Btrfs/XFS). On ext4 without reflink, NTFS, or FAT32, BranchBase falls back to chunked file copying. Additionally, active WAL sidecars (`-wal`, `-shm`) require checkpointing (`PRAGMA wal_checkpoint(TRUNCATE)`) before safe snapshots.
- **MongoDB Collection Cloning:** MongoDB cloning uses `$out` aggregation pipelines across collections and reconstructs indexes. Capped collections and time-series collections require standard collection semantics.

---

## Architecture: Adding a Database Driver

External engines are pluggable by design. Adding a new database driver requires just 1 package and 1 interface implementation:

```go
// internal/driver/driver.go
type Driver interface {
    Name() string
    Ping(ctx context.Context) error
    BranchExists(ctx context.Context, branchName string) (bool, error)
    CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error
    DeleteBranch(ctx context.Context, branchName string) error
    ListBranches(ctx context.Context) ([]BranchInfo, error)
    Close() error
}
```

1. Create `internal/driver/<engine>/<engine>.go`.
2. Implement the `Driver` interface.
3. Register your factory via `driver.Register("<engine>", factory)` in `init()`.
4. See our dedicated [Driver Development Skill](.agents/skills/branchbase-driver/SKILL.md) for full instructions.

---

## Framework Integration Guides

- **[Prisma ORM Integration Guide](docs/guides/prisma.md)**: Zero-conflict database migrations with TypeScript & Node.js.

## Quickstart

### 1. Initialize in your Repository
```bash
cd my-awesome-project
branchbase init
```

### 2. Start the Transparent Proxy
```bash
branchbase proxy
```

### 3. Work with Git as you always do!
```bash
# Branch to a new feature:
git checkout -b feature/stripe-billing

# Run migrations freely:
npx prisma migrate dev  # or rails db:migrate / alembic upgrade head

# Switch back to main whenever you want:
git checkout main
# Proxy immediately routes traffic back to your main database! No migration errors!
```

### 4. Inspect Branch Status
```bash
# Human-readable summary
branchbase status

# Machine-readable JSON for prompt scripts, CI/CD, or status bars
branchbase status --json
```

---

## Contributing & Community

Thinking about contributing? We'd love to have you!

- **New Contributors:** Check our [`good first issue`](https://github.com/oscarbol09/branchbase/labels/good%20first%20issue) label for onboarding tasks.
- **Contributor Guide:** Read [CONTRIBUTING.md](CONTRIBUTING.md) for coding standards, Conventional Commits, and PR rules.
- **Environment Setup:** See [SETUP.md](SETUP.md) for local Go and Docker development steps.
- **Code of Conduct:** All interactions are governed by our [Code of Conduct](CODE_OF_CONDUCT.md).

---

## Security

To report a vulnerability privately, please see [SECURITY.md](SECURITY.md) or use [GitHub Private Vulnerability Reporting](https://github.com/oscarbol09/branchbase/security/advisories/new).

---

## Support & Sponsorship

If you find **BranchBase** useful in your daily development or it saved you hours of debugging migration mismatches, consider supporting ongoing development:

- **[Sponsor on GitHub Sponsors](https://github.com/sponsors/oscarbol09)**
- **[Support on Ko-Fi](https://ko-fi.com/oscarmb09)**

Your sponsorship helps fund test infrastructure, multi-database driver maintenance, and cross-platform packaging.

---

## License

Licensed under the [MIT License](LICENSE).
