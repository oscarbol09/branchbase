# Reddit Launch Strategy & Copy Kit 🤖

> **Rules of Reddit:**
> 1. Never post the same text across multiple subreddits on the same day.
> 2. Space submissions across 2–3 days.
> 3. Participate actively in comments and answer every technical question.
> 4. Keep the tone authentic: you are an engineer sharing a project you built to solve your own pain point.

---

## 1. r/golang (High Intent & Go Ecosystem)

* **Post Type:** Text Post
* **Title:**  
  `I built BranchBase: A pure-Go local database branching tool and transparent proxy for PostgreSQL, MySQL & SQLite`
* **Body:**

```text
Hey Gophers!

Over the past few months, I got frustrated with the constant friction between Git branches and local databases. Switching between `feature/X` (with new schema migrations) and `main` constantly broke my local environment, causing ActiveRecord/Prisma migration mismatch errors and forcing me to drop Docker volumes.

So I built **BranchBase** in Go: https://github.com/oscarbol09/branchbase

### What it does:
- Hooks into `post-checkout` / `post-merge` to detect branch switches in < 5ms (reads `.git/HEAD` directly).
- Takes instant zero-copy snapshots:
  - PostgreSQL: `CREATE DATABASE ... TEMPLATE`
  - MySQL / MariaDB: Schema cloning with transactional replication
  - SQLite: Copy-on-Write snapshots (APFS `clonefile` on macOS, `FICLONE` reflink on Linux Btrfs/XFS)
- Runs a transparent TCP routing proxy in Go: Intercepts port 5432, parses PostgreSQL `StartupMessage` wire protocol, rewrites the target database to the active branch, and streams bidirectionally with zero overhead. Your app's `DATABASE_URL` never changes.
- Includes a pure-Go ANSI Terminal UI (`branchbase tui`) with arrow key navigation and real-time database inspection.
- Auto-detects Docker Compose files and port mappings on `branchbase init`.

The codebase is 100% standard library where possible (net, database/sql, os), MIT licensed, and fully tested across Linux, macOS, and Windows with a live PG 16 integration CI suite.

Would love feedback from other Go developers, especially on the wire-protocol parser and connection draining logic!
```

---

## 2. r/selfhosted (Privacy & Local-First Advocates)

* **Post Type:** Text Post
* **Title:**  
  `BranchBase – Open-source, local-first database branching for Docker & local development (No cloud, 100% offline)`
* **Body:**

```text
Hi everyone!

If you use self-hosted Docker containers for your databases during development, you might find this useful.

I built **BranchBase** (https://github.com/oscarbol09/branchbase) — a tool that brings Git-like branching to your local PostgreSQL, MySQL, and SQLite databases.

Instead of paying for cloud branching services or dealing with broken local databases whenever you switch Git branches, BranchBase:
1. Detects `git checkout` via hooks.
2. Automatically creates an isolated snapshot of your database for that branch.
3. Transparently proxies queries on `localhost:5432` so your app always connects to the active branch's database without editing `.env`.
4. Automatically cleans up (`branchbase prune`) when branches are merged or deleted.

It runs completely on your machine, requires zero internet connection, sends zero telemetry, and is MIT licensed.

Feedback and suggestions are warmly welcomed!
```

---

## 3. r/PostgreSQL (Technical Postgres Community)

* **Post Type:** Text Post
* **Title:**  
  `Zero-latency local PostgreSQL database branching using CREATE DATABASE TEMPLATE and wire-protocol packet rewriting`
* **Body:**

```text
Hi Postgres community!

I wanted to share a developer tool I've been working on to solve the local schema migration conflict problem when switching Git branches.

Repo: https://github.com/oscarbol09/branchbase

### How we leverage PostgreSQL internals:
1. **Instant Cloning:** When switching branches, BranchBase disconnects lingering connections using `pg_terminate_backend(pid)` and provisions the branch database using `CREATE DATABASE target TEMPLATE source;`.
2. **Transparent Wire Proxy:** Instead of making developers change `DATABASE_URL` in `.env`, BranchBase listens on port 5432, intercepts the initial PostgreSQL `StartupMessage` packet, rewrites the `database` parameter to the active branch name, handles SSL negotiation (`sslmode=require` / self-signed TLS upgrade), and forwards traffic to the backend server with graceful connection draining.
3. **Safety:** Base databases (like `main`) are strictly protected from accidental deletion.

I'd appreciate any feedback on edge cases in `pgwire` packet rewriting or connection handling!
```
