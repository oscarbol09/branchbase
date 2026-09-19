# Contributing to BranchBase 🌿

First off, thank you for considering contributing to **BranchBase**! Projects like this thrive because of developers like you who care about improving day-to-day developer experience.

Whether you want to write code, design new database drivers, report bugs, improve documentation, or share feedback, your help is warmly welcomed.

---

## 🌱 New to the Project? Start Here!

If you are looking for approachable tasks to get started, check our curated newcomer issues:
- **[Good First Issues on GitHub](https://github.com/oscarbol09/branchbase/labels/good%20first%20issue)**
- **[BranchBase on Up For Grabs](https://up-for-grabs.net/)**

Each newcomer issue includes exact file pointers, reproduction steps, and expected behavior. Maintainers are actively available to guide you through your first PR!

---

## 🎯 How Can You Contribute?

Here are some high-impact areas where we need collaboration:

1. **Database Drivers:**
   - ✅ PostgreSQL Driver (`CREATE DATABASE ... TEMPLATE`) — *Implemented.*
   - ✅ SQLite Driver (Reflink / APFS `clonefile` / Btrfs / XFS) — *Implemented.*
   - ✅ MySQL / MariaDB Driver (`CREATE TABLE ... LIKE` schema cloning) — *Implemented.*
   - 🔧 MongoDB Driver — *Open for contribution.*
   - 🔧 CockroachDB / TiDB Driver — *Open for contribution.*
2. **Transparent Proxy Engine:**
   - ✅ Wire-protocol parsing for PostgreSQL `StartupMessage` — *Implemented.*
   - ✅ TLS/SSL negotiation (`sslmode=require`) — *Implemented.*
   - ✅ UNIX domain socket support — *Implemented.*
   - ✅ Graceful connection draining — *Implemented.*
   - 🔧 Wire-protocol parsing for MySQL Handshake — *Open for contribution.*
3. **CLI & Developer Experience:**
   - ✅ Git hook installation & integration tests — *Implemented.*
   - ✅ Interactive TUI (Terminal UI) dashboard — *Implemented.*
   - ✅ Autocompletion scripts (bash, zsh, fish, powershell) — *Implemented.*
   - 🔧 `branchbase doctor` diagnostic command — *Open for contribution.*
4. **Documentation & Guides:**
   - ✅ Prisma ORM integration guide — *Implemented.*
   - 🔧 Integration guides for: **Drizzle**, **Django**, **Ruby on Rails**, **Alembic/SQLAlchemy**, **TypeORM**.

---

## 🛠️ Development Setup

### Prerequisites
- **Go** (1.22+ or latest stable).
- **Docker & Docker Compose** (for running local test databases).
- **Git** (2.30+).

### Clone & Build
```bash
git clone https://github.com/oscarbol09/branchbase.git
cd branchbase

# Download dependencies
go mod download

# Run local tests
go test ./...

# Build the CLI binary
go build -o bin/branchbase ./cmd/branchbase
```

---

## 🧩 Implementing a New Database Driver

To add support for a new database, implement the `Driver` interface defined in `internal/driver/driver.go`:

```go
package driver

import "context"

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

1. Create a new package under `internal/driver/<engine>/`.
2. Implement all interface methods.
3. Add unit and integration tests.
4. Register the driver via `driver.Register("<engine>", factory)` in `init()`.
5. Open a Pull Request!

> [!NOTE]
> **Optional Connection Pool Parameters:**  
> The factory function `func(params map[string]interface{}) (Driver, error)` receives configuration values from `.branchbase.yaml`. For short-lived operations (such as `hook-trigger`), BranchBase passes lightweight pool limits into `params`:
> - `max_open_conns` (`int`): Maximum concurrent open database connections (e.g., `1` in lightweight mode).
> - `max_idle_conns` (`int`): Maximum idle connections retained in the pool (e.g., `1` in lightweight mode).  
> 
> Database drivers supporting connection pooling should inspect these parameters and apply them to their connection pool.

---

## 📋 Pull Request Guidelines

1. **Keep it Focused:** A pull request should do one thing well. Avoid bundling unrelated refactors.
2. **Write Tests:** If you are fixing a bug or adding a feature, please include automated tests.
3. **Conventional Commits:** We follow the [Conventional Commits](https://www.conventionalcommits.org/) convention:
   - `feat: add postgres startup packet rewriter`
   - `fix: handle detached HEAD state in git resolver`
   - `docs: update quickstart guide for Rails users`
4. **Documentation:** Update the `README.md` or `ARCHITECTURE.md` if your change modifies user-facing behavior.

---

## 💬 Community & Questions

- **GitHub Discussions:** Use Discussions for architecture proposals, RFCs, and questions.
- **Issues:** Use GitHub Issues for bug reports and tracked feature requests.

Thank you for building the future of local-first database developer experience with us! 🚀
