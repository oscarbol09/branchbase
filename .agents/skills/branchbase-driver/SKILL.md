---
name: branchbase-driver
description: >-
  Provides end-to-end instructions, templates, and testing patterns for implementing
  new database engine drivers (PostgreSQL, MySQL, SQLite, MongoDB) in BranchBase.
---

# BranchBase Database Driver Development Skill 🌿

Use this skill whenever you are adding, debugging, or refactoring a database engine driver inside BranchBase.

---

## 1. The Driver Interface Contract

Every driver must reside in `internal/driver/<engine>/` and implement the `driver.Driver` interface from `internal/driver/driver.go`:

```go
type Driver interface {
    Name() string
    Ping(ctx context.Context) error
    BranchExists(ctx context.Context, branchName string) (bool, error)
    CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error
    DeleteBranch(ctx context.Context, branchName string) error
    ListBranches(ctx context.Context) ([]BranchInfo, error)
}
```

---

## 2. Implementation Guidelines by Engine

### A. PostgreSQL
- **Mechanism:** `CREATE DATABASE <target> TEMPLATE <source>;`
- **Pre-requisite:** Terminate active connections to the template database using `pg_terminate_backend(pid)` before issuing the `TEMPLATE` command.
- **Identifier Rules:** PostgreSQL database names are limited to 63 bytes. Ensure `git.SanitizeBranchName()` output is truncated safely if needed.

### B. SQLite
- **Mechanism:** Fast Copy-on-Write (CoW) / Reflink:
  - macOS: `sys/unix.Clonefile()` on APFS.
  - Linux: `unix.IoctlFileClone()` on Btrfs/XFS.
  - Windows: Fast streaming file copy with buffer pooling.
- **Pre-requisite:** Issue `PRAGMA wal_checkpoint(TRUNCATE);` before taking a snapshot so the `.db-wal` file is synced into the primary `.db` file.

### C. MySQL / MariaDB
- **Mechanism:** 
  - For small databases: `mysqldump --no-data` + pipe into target database, followed by data insert.
  - For Docker volume setups: volume snapshot via Docker API.

---

## 3. Driver Registration Workflow

1. In your driver package `internal/driver/<engine>/<engine>.go`, add an `init()` function:
   ```go
   func init() {
       driver.Register("<engine>", func(params map[string]interface{}) (driver.Driver, error) {
           // parse params and return Driver instance
       })
   }
   ```
2. Blank-import the driver in `cmd/branchbase/main.go` so it gets registered:
   ```go
   _ "github.com/branchbase/branchbase/internal/driver/<engine>"
   ```

---

## 4. Verification Checklist

- [ ] All methods take `context.Context` and handle cancellation.
- [ ] Safe quoting of SQL identifiers (e.g. `fmt.Sprintf("CREATE DATABASE %q", dbName)`).
- [ ] Base database (`main`) is protected and can never be deleted by `DeleteBranch`.
- [ ] Unit tests added with table-driven test cases.
