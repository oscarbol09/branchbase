# BranchBase 🌿

> **Zero-config, Git-native local database branching for PostgreSQL, MySQL, and SQLite.**  
> Stop dropping your local database every time you switch Git branches.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Status: RFC & Active Development](https://img.shields.io/badge/Status-RFC%20%2F%20v0.1.0--alpha-blue.svg)](#roadmap)

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

## 🛠️ Quickstart

### 1. Installation (Upcoming)
```bash
# via Homebrew (macOS/Linux)
brew install branchbase/tap/branchbase

# via Go
go install github.com/branchbase/branchbase/cmd/branchbase@latest

# via Cargo (Rust)
cargo install branchbase
```

### 2. Initialize in your Repository
Navigate to your project root (where your `.git` and `docker-compose.yml` live):

```bash
cd my-awesome-project
branchbase init
```

This will interactively detect your local database configuration and generate `.branchbase.yaml`:

```yaml
version: "1"
driver: postgres
connection:
  host: "127.0.0.1"
  port: 5433         # Real underlying Postgres port (Docker)
  user: "postgres"
  password: "password"
  base_database: "myapp_dev"

proxy:
  listen_port: 5432   # App points here!
  default_branch: "main"

strategy:
  snapshot_on_switch: true
  auto_prune_merged: true
```

### 3. Start the Transparent Proxy
```bash
branchbase proxy start
```

### 4. Work with Git as you always do!
```bash
# You are on main with 1,000 seeded rows
git checkout -b feature/stripe-billing

# BranchBase instantly creates `myapp_dev_feature_stripe_billing` from `myapp_dev`
# Now run your new migrations:
npx prisma migrate dev  # or rails db:migrate / alembic upgrade head

# Switch back to main:
git checkout main
# Proxy immediately routes traffic back to `myapp_dev`! No migration errors!
```

---

## 🧩 Supported Databases

| Engine | Branching Mechanism | Status |
| :--- | :--- | :--- |
| **PostgreSQL** | `CREATE DATABASE ... TEMPLATE` / CoW | 🟡 Active MVP |
| **SQLite** | Reflink / Fast File Copy / WAL checkpoint | 🟡 Active MVP |
| **MySQL / MariaDB** | Schema dump + Data piping / Docker Volume Snapshot | ⚪ Planned (v0.2) |
| **MongoDB** | Ephemeral DB namespaces | ⚪ Exploring |

---

## 🗺️ Project Roadmap & Milestones

- [x] **Phase 0:** Problem validation, RFC, architectural design, community alignment.
- [ ] **Phase 1 (MVP):**
  - [ ] Git branch context resolver (`HEAD` watcher).
  - [ ] PostgreSQL engine driver (`TEMPLATE` based cloning).
  - [ ] Transparent TCP proxy for Postgres protocol routing.
  - [ ] CLI commands: `init`, `status`, `switch`, `list`, `prune`.
- [ ] **Phase 2:**
  - [ ] SQLite driver with APFS/Btrfs CoW and cross-platform fallback.
  - [ ] Automatic Docker Compose integration (auto-detect exposed ports).
  - [ ] Terminal UI (TUI) via `bubbletea` or `ratatui`.
- [ ] **Phase 3:**
  - [ ] MySQL / MariaDB engine driver.
  - [ ] Seed data sharing between branches without re-migrating.

See our [ROADMAP.md](ROADMAP.md) for full technical milestones.

---

## 🤝 Contributing & Community

BranchBase is being built **in public from day zero**. We are actively looking for:
* **Core maintainers and contributors** (Go / Rust / Database internals).
* **Database Driver authors** (Postgres, MySQL, SQLite, MongoDB).
* **Testing volunteers** across different ORMs (Prisma, Drizzle, Django, Rails, Hibernate, SQLAlchemy).

Read our [CONTRIBUTING.md](CONTRIBUTING.md) to get started. Don't hesitate to open an issue or start a Discussion!

---

## 📄 License

Licensed under the [MIT License](LICENSE).
