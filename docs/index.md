# BranchBase 🌿

> **Zero-config, Git-native local database branching for PostgreSQL, MySQL, and SQLite.**  
> Stop dropping your local database every time you switch Git branches.

---

<p align="center">
  <img src="assets/terminal-demo.svg" alt="BranchBase Terminal Demo" width="800">
</p>

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
