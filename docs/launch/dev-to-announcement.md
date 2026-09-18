# DEV.to / Hashnode Announcement Article 📝

> **Target Platforms:** [DEV.to](https://dev.to) & [Hashnode](https://hashnode.com)  
> **Tags:** `go`, `database`, `postgresql`, `opensource`, `devtools`  
> **Cover Image:** `assets/banner.svg` or `assets/social-preview.svg`

---

```markdown
---
title: "Stop Dropping Your Local Database on Git Checkout: Introducing BranchBase 🌿"
published: true
description: "Zero-config, Git-native local database branching for PostgreSQL, MySQL, and SQLite. Never suffer migration mismatch crashes again."
tags: go, database, postgresql, opensource
cover_image: https://raw.githubusercontent.com/oscarbol09/branchbase/main/assets/banner.svg
canonical_url: https://github.com/oscarbol09/branchbase
---

If you develop backend applications with Docker, local PostgreSQL, MySQL, or SQLite, you've probably suffered this loop at least once this week:

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

### The Usual Workarounds (and why they hurt)

- **The Nuclear Option:** `docker compose down -v && docker compose up -d`  
  *(You lose all your mock data, user logins, and test seeds. Takes 5 minutes to re-seed).*
- **The Manual Rollback:** Trying to step backward through migrations on your branch before switching.
- **The `.env` Nightmare:** Manually maintaining 5 different connection strings in `.env` and constantly changing ports.
- **Cloud Branching (Neon, PlanetScale):** Excellent DX, but proprietary, paid, and useless if you are offline on a train or plane.

---

## Introducing BranchBase 🌿

**BranchBase** brings instant, zero-copy database branching directly to your **local machine and Docker containers**.

![BranchBase Terminal Demo](https://raw.githubusercontent.com/oscarbol09/branchbase/main/assets/terminal-demo.svg)

### How It Works Under the Hood

1. **Detection:** When you run `git checkout <branch>`, BranchBase's hook (`.git/hooks/post-checkout`) detects the branch transition in under 5ms.
2. **Copy-on-Write Snapshot:**
   - **PostgreSQL:** Disconnects lingering connections to the template and executes `CREATE DATABASE <target> TEMPLATE <source>;` in milliseconds.
   - **MySQL / MariaDB:** Clones schemas and tables with transactional safety.
   - **SQLite:** Uses filesystem Copy-on-Write (`clonefile` on macOS APFS, `FICLONE` on Linux).
3. **Transparent Wire Proxy:** Your application continues querying `localhost:5432`. BranchBase's transparent TCP proxy inspects your active Git branch and rewrites the database packet on the fly. **Your `.env` file never changes.**
4. **Interactive Dashboard:** Run `branchbase tui` to view, switch, and inspect branch database sizes with zero dependencies.
5. **Lifecycle Pruning:** Run `branchbase prune` to clean up merged or orphaned branch databases.

---

## 🚀 Quickstart in 60 Seconds

### 1. Install BranchBase

```bash
# macOS / Linux via Homebrew
brew install oscarbol09/branchbase/branchbase

# or via Go
go install github.com/oscarbol09/branchbase/cmd/branchbase@latest
```

### 2. Initialize in your Repository

```bash
cd my-project
branchbase init
```

`branchbase init` automatically detects your `docker-compose.yml`, configures your database engine, and installs Git hooks.

### 3. Start the Transparent Proxy

```bash
branchbase proxy
```

### 4. Work with Git as you normally do!

```bash
# Create a new feature branch
git checkout -b feature/stripe-billing

# Run your ORM migrations freely
npx prisma migrate dev  # or rails db:migrate / alembic upgrade head

# Switch back to main
git checkout main
# Proxy immediately routes traffic back to your main database! No migration errors!
```

---

## 💚 100% Local, Offline & Open-Source

BranchBase is licensed under MIT, sends zero telemetry, requires no cloud account, and works completely offline.

- ⭐ **GitHub Repository:** [github.com/oscarbol09/branchbase](https://github.com/oscarbol09/branchbase)
- 📖 **Documentation:** [oscarbol09.github.io/branchbase](https://oscarbol09.github.io/branchbase/)

If this tool solves a daily pain point for you, please consider giving it a ⭐ on GitHub!
```
