# Contributing to BranchBase 🌿

First off, thank you for considering contributing to **BranchBase**! Projects like this thrive because of developers like you who care about improving day-to-day developer experience.

Whether you want to write code, design new database drivers, report bugs, improve documentation, or share feedback, your help is warmly welcomed.

---

## 🎯 How Can You Contribute?

Here are some high-impact areas where we need collaboration:

1. **Database Drivers:**
   - PostgreSQL Driver (`CREATE DATABASE ... TEMPLATE`).
   - SQLite Driver (Reflink / APFS `clonefile` / Btrfs / XFS).
   - MySQL / MariaDB Driver.
   - MongoDB Driver.
2. **Transparent Proxy Engine:**
   - Wire-protocol parsing for PostgreSQL `StartupMessage`.
   - Wire-protocol parsing for MySQL Handshake.
3. **CLI & Developer Experience:**
   - Git hook installation & integration tests.
   - Interactive TUI (Terminal UI) for managing and inspecting branch databases.
   - Autocompletion scripts (bash, zsh, fish).
4. **Documentation & Guides:**
   - Integration guides with popular ORMs: **Prisma**, **Drizzle**, **Django**, **Ruby on Rails**, **Alembic/SQLAlchemy**, **TypeORM**.

---

## 🛠️ Development Setup

### Prerequisites
- **Go** (1.22+ or latest stable) or **Rust** (depending on the core component).
- **Docker & Docker Compose** (for running local test databases).
- **Git** (2.30+).

### Clone & Build
```bash
git clone https://github.com/branchbase/branchbase.git
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
}
```

1. Create a new package under `internal/driver/<engine>/`.
2. Implement all interface methods.
3. Add unit and integration tests using `testcontainers-go`.
4. Register the driver in `internal/driver/registry.go`.
5. Open a Pull Request!

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
