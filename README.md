# BranchBase 🌿

> **Zero-config, Git-native local database branching for PostgreSQL, MySQL, and SQLite.**  
> Stop dropping your local database every time you switch Git branches.

[![BranchBase CI](https://github.com/oscarbol09/branchbase/actions/workflows/ci.yml/badge.svg)](https://github.com/oscarbol09/branchbase/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Good First Issues](https://img.shields.io/github/issues/oscarbol09/branchbase/good%20first%20issue?color=7057ff&label=good%20first%20issues)](https://github.com/oscarbol09/branchbase/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
[![GitHub Sponsors](https://img.shields.io/badge/Sponsor-GitHub-ea4aaa?logo=github-sponsors&logoColor=white)](https://github.com/sponsors/oscarbol09)
[![Support on Ko-Fi](https://img.shields.io/badge/Support-Ko--Fi-F16061?logo=ko-fi&logoColor=white)](https://ko-fi.com/oscarmb09)

---

## ⚡ The Problem: The "Git vs. Local Database" Friction

Every developer working with Docker, local PostgreSQL, MySQL, or SQLite has suffered this loop:

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

## 🚀 The Solution: BranchBase

**BranchBase** brings instant, zero-copy database branching directly to your **local machine and Docker containers**.

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
          (Detects new branch)                      (Zero-Copy Snapshot)
                    |                                         |
                    v                                         v
+---------------------------------------+   +------------------------------------+
|       BranchBase Proxy (Port 5432)    |   |     Local Database (PostgreSQL)    |
+---------------------------------------+   +------------------------------------+
| - App connection string NEVER changes |   | - `db_project_main` (frozen)       |
| - Automatically routes queries to the |   | - `db_project_branch_b` (active)   |
|   active Git branch database!         |   |   (Created instantly via TEMPLATE) |
+---------------------------------------+   +------------------------------------+
```

### Key Highlights
- ⚡ **Instant Branching:** Creates a fresh, isolated branch database in milliseconds using PostgreSQL `CREATE DATABASE ... TEMPLATE` or filesystem copy-on-write (reflink/APFS/Btrfs for SQLite).
- 🔌 **Transparent Connection Proxy:** Your app's `DATABASE_URL=postgres://user:pass@localhost:5432/myapp` **never changes**. The local proxy automatically inspects which Git branch is active in your working directory and routes traffic to that branch's database.
- 🎣 **Automated Git Hook:** Hooks into `post-checkout` and `post-merge`. You simply use standard `git checkout` or `git switch`.
- 🧹 **Automatic Cleanup (`prune`):** When you delete or merge a Git branch, `branchbase` safely tears down the associated ephemeral database.
- 📴 **100% Local & Offline:** No cloud telemetry, no subscription fees, no internet needed.

---

## 📖 Command & CLI Reference

| Command | What it does |
| :--- | :--- |
| `branchbase init` | Interactively inspect repository and generate `.branchbase.json` |
| `branchbase proxy` | Start the local transparent TCP routing proxy (default port: `5432`) |
| `branchbase status` | Display the active Git branch, sanitized name, target DB, and driver status |
| `branchbase list` | List all active and ephemeral databases managed by BranchBase with disk usage |
| `branchbase switch <branch>` | Manually switch or provision an isolated database for a specific branch |
| `branchbase prune` | Detect and delete databases associated with merged or deleted Git branches |
| `branchbase version` | Print the current BranchBase version |

---

## 📁 Repository Structure

```text
branchbase/
├── cmd/
│   └── branchbase/
│       └── main.go               # CLI entry point (subcommands & signal handling)
├── internal/
│   ├── config/                   # Configuration loader (.branchbase.json / .yaml)
│   ├── driver/                   # Database engine interfaces & registry
│   │   ├── driver.go             # Core Driver interface contract
│   │   ├── postgres/             # PostgreSQL engine (TEMPLATE cloning)
│   │   └── sqlite/               # SQLite engine (CoW / Reflink snapshots)
│   ├── git/                      # Git HEAD inspector and branch sanitization
│   │   ├── resolver.go           # Non-subshell .git/HEAD resolution
│   │   └── resolver_test.go      # Table-driven unit test suite
│   └── proxy/                    # Transparent TCP proxy & wire routing
│       └── proxy.go              # Zero-overhead bidirectional TCP forwarder
├── .agents/                      # Custom Agent skills & development workflows
├── .github/                      # CI workflows, issue templates, dependabot
├── ARCHITECTURE.md               # Detailed system design & sequence diagrams
├── CONTRIBUTING.md               # Contributor guide & driver creation tutorial
├── SETUP.md                      # Local developer environment setup guide
├── SECURITY.md                   # Security policy & private vulnerability reporting
├── CODE_OF_CONDUCT.md            # Contributor Covenant v2.1
├── CHANGELOG.md                  # Keep a Changelog version history
├── branchbase.example.yaml       # Annotated configuration specification
└── go.mod                        # Go 1.22+ module definition
```

---

## ⚙️ How It Works (Step-by-Step)

1. **Detection:** When you run `git checkout <branch>`, BranchBase's hook (`.git/hooks/post-checkout`) detects the branch transition in under 5ms by reading `.git/HEAD`.
2. **Identifier Sanitization:** Special characters like `/` or `-` in branch names (e.g. `feature/stripe-v2`) are converted into safe database identifiers (`feature_stripe_v2`).
3. **Copy-on-Write Snapshot:**
   * **PostgreSQL:** Disconnects lingering connections to the template and executes `CREATE DATABASE <target> TEMPLATE <source>;` (instant CoW clone).
   * **SQLite:** Issues `PRAGMA wal_checkpoint(TRUNCATE);` and performs a filesystem reflink/clone (`clonefile()` or `FICLONE`).
4. **Transparent Routing:** When your backend app queries `localhost:5432`, the BranchBase proxy intercepts the connection, resolves the active branch database, and forwards traffic seamlessly.
5. **Lifecycle Pruning:** Once a PR is merged into `main`, running `branchbase prune` removes the ephemeral database, freeing disk space.

---

## 🧩 Extension Model: Adding a New Database Driver

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
}
```

1. Create `internal/driver/<engine>/<engine>.go`.
2. Implement the `Driver` interface.
3. Register your factory via `driver.Register("<engine>", factory)` in `init()`.
4. See our dedicated [Driver Development Skill](.agents/skills/branchbase-driver/SKILL.md) for full instructions.

---

## 🛠️ Quickstart

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

---

## 🤝 Contributing & Community

Thinking about contributing? We'd love to have you!

- **New Contributors:** Check our [`good first issue`](https://github.com/oscarbol09/branchbase/labels/good%20first%20issue) label for onboarding tasks.
- **Contributor Guide:** Read [CONTRIBUTING.md](CONTRIBUTING.md) for coding standards, Conventional Commits, and PR rules.
- **Environment Setup:** See [SETUP.md](SETUP.md) for local Go and Docker development steps.
- **Code of Conduct:** All interactions are governed by our [Code of Conduct](CODE_OF_CONDUCT.md).

---

## 🛡️ Security

To report a vulnerability privately, please see [SECURITY.md](SECURITY.md) or use [GitHub Private Vulnerability Reporting](https://github.com/oscarbol09/branchbase/security/advisories/new).

---

## 💖 Support & Sponsorship

If you find **BranchBase** useful in your daily development or it saved you hours of debugging migration mismatches, consider supporting ongoing development:

- 💖 **[Sponsor on GitHub Sponsors](https://github.com/sponsors/oscarbol09)**
- ☕ **[Buy me a coffee on Ko-Fi](https://ko-fi.com/oscarmb09)**

Your sponsorship helps fund test infrastructure, multi-database driver maintenance, and cross-platform packaging!

---

## 📄 License

Licensed under the [MIT License](LICENSE).
