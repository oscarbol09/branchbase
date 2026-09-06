# BranchBase Technical Architecture 🏗️

This document outlines the internal architecture, design principles, and lifecycle flow of **BranchBase**.

---

## 1. High-Level System Architecture

BranchBase sits between the developer's Git workflow, their application code, and the underlying database instance (typically running in Docker or natively).

```mermaid
graph TD
    subgraph Developer Workspace
        Git[Git CLI / IDE] -->|checkout / switch| GitHook[BranchBase Git Hook\n.git/hooks/post-checkout]
        App[Application Backend\ne.g., Next.js, Rails, FastAPI] -->|Queries to Port 5432| Proxy[BranchBase Transparent Proxy]
    end

    subgraph BranchBase Core
        GitHook -->|Trigger event| CoreDaemon[BranchBase Core Engine]
        Proxy -->|Query Router| CoreDaemon
        CoreDaemon -->|Branch Resolution| GitResolver[Git Head Resolver]
        CoreDaemon -->|Snapshot / Clone| DBDriver[Database Engine Driver]
    end

    subgraph Database Layer [Docker / Native]
        DBDriver -->|CREATE DATABASE TEMPLATE| PG[(PostgreSQL)]
        DBDriver -->|Reflink / CoW Copy| SQLite[(SQLite)]
        DBDriver -->|Volume / DB Clone| MySQL[(MySQL)]
    end
```

---

## 2. Core Components

### 2.1 The Git Context Resolver (`internal/git`)
* **Responsibility:** Determines the currently active branch in the repository root without executing expensive subshells.
* **Mechanism:** 
  1. Inspects `.git/HEAD`.
  2. If `HEAD` is a symbolic ref (`ref: refs/heads/feature-name`), resolves the branch name directly.
  3. Sanitizes branch names for database naming compatibility (e.g. `feature/stripe-v2` $\rightarrow$ `feature_stripe_v2`).

### 2.2 The Database Driver Interface (`internal/driver`)
Every supported database implements a standard Go/Rust interface:

```go
type Driver interface {
    // Name returns the driver identifier (e.g., "postgres", "sqlite")
    Name() string

    // Ping verifies connectivity to the underlying database engine
    Ping(ctx context.Context) error

    // BranchExists checks if a database for the given branch already exists
    BranchExists(ctx context.Context, branchName string) (bool, error)

    // CreateBranch clones sourceBranch into targetBranch
    CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error

    // DeleteBranch tears down the ephemeral database
    DeleteBranch(ctx context.Context, branchName string) error

    // ListBranches returns all databases managed by BranchBase
    ListBranches(ctx context.Context) ([]BranchInfo, error)
}
```

#### PostgreSQL Implementation:
Postgres natively supports instant database cloning via the `TEMPLATE` directive:
```sql
-- Step 1: Disconnect any active connections to template (if needed)
SELECT pg_terminate_backend(pid) FROM pg_stat_activity 
WHERE datname = 'myapp_dev_main' AND pid <> pg_backend_pid();

-- Step 2: Instant copy-on-write clone
CREATE DATABASE myapp_dev_feature_billing TEMPLATE myapp_dev_main;
```

#### SQLite Implementation:
For SQLite, BranchBase utilizes filesystem-level **Copy-on-Write (CoW)** where supported:
* **macOS (APFS):** `clonefile()` syscall (instant 0-byte clone).
* **Linux (Btrfs / XFS):** `ioctl(FICLONE)` reflink copying.
* **Windows (ReFS / NTFS fallback):** Hardlink or optimized fast stream copy.

---

### 2.3 The Transparent TCP Proxy (`internal/proxy`)
To ensure developers **never have to touch `.env`** or restart their dev servers when switching branches:

1. The proxy listens on the standard port (e.g., `5432` for Postgres).
2. The real database container is remapped to an internal port (e.g., `5433` or a UNIX domain socket).
3. The proxy intercepts the initial connection handshake:
   * **PostgreSQL StartupPacket:** The client sends the requested database name in the startup message (`StartupMessage` packet).
   * The proxy replaces the target database name with the branch-specific database name (e.g., rewriting `myapp_dev` to `myapp_dev_feature_billing`).
   * Forwards the modified byte stream to the underlying database.
   * From that point forward, acts as a high-performance, zero-overhead bidirectional TCP forwarder.

---

## 3. The Lifecycle of a Branch Switch

```mermaid
sequenceDiagram
    autonumber
    actor Dev as Developer
    participant Git as Git CLI
    participant Hook as post-checkout Hook
    participant BB as BranchBase Daemon
    participant DB as Postgres Instance
    participant Proxy as BranchBase Proxy

    Dev->>Git: git checkout -b feature/auth
    Git->>Hook: Trigger post-checkout(old_head, new_head, 1)
    Hook->>BB: Notify branch_switch("main", "feature/auth")
    BB->>DB: Check if db_feature_auth exists
    alt Database does not exist
        BB->>DB: CREATE DATABASE db_feature_auth TEMPLATE db_main;
        DB-->>BB: OK (Instant)
    end
    BB->>Proxy: Update active_branch_routing("feature/auth")
    Proxy-->>Dev: Ready!

    Note over Dev,Proxy: App sends query: "SELECT * FROM users;"
    Proxy->>DB: Forwarded to db_feature_auth transparently
```

---

## 4. Branch Cleanup & Pruning

Over time, working on dozens of feature branches can accumulate disk space.

* **Command:** `branchbase prune`
* **Algorithm:**
  1. Inspects local and remote Git branches using `git branch --merged` and `git for-each-ref`.
  2. Identifies all databases matching the pattern `<base_db>_<branch>`.
  3. If the branch has been merged into the default branch (`main`/`master`) or deleted, the database is marked for deletion.
  4. Drops the ephemeral databases, freeing all allocated storage.

---

## 5. Security & Isolation Guarantees

* **Zero Cloud Dependency:** BranchBase runs strictly on `localhost` or via private UNIX domain sockets. No data or telemetry leaves the machine.
* **Non-Destructive:** BranchBase **never** modifies or drops the `base_database` (e.g., `main`). Default branches are marked as protected by default.
* **Ephemeral Workspaces:** Can be integrated into Docker Compose workflows via named volumes.
