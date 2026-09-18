# Twitter / X Launch Thread 🧵

> **Platform:** [x.com](https://x.com)  
> **Media to Attach to Tweet 1:** `assets/terminal-demo.svg` or `assets/banner.svg`

---

### Tweet 1 (Hook + Media)
```text
🌿 Introducing BranchBase: Git-native, zero-config local database branching for PostgreSQL, MySQL, and SQLite.

Never drop your local Docker database or crash from migration mismatches on branch switch again.

100% local. Zero cloud. MIT licensed.

👇 Here is how it works:
```

---

### Tweet 2 (The Problem)
```text
The friction every developer knows:
1. You work on `feature/stripe-billing` (ran migrations).
2. Bug alert! You `git checkout main`.
3. 💥 CRASH: "column billing_tier does not exist".

Your options today?
- docker compose down -v (loses all seeds)
- Manual rollback dances
- Paid cloud platforms
```

---

### Tweet 3 (The Solution & Transparent Proxy)
```text
BranchBase brings instant Copy-on-Write snapshots to your local machine:

⚡ Sub-millisecond branch detection via .git/hooks
🔌 Transparent TCP wire-level proxy: your app's DATABASE_URL NEVER changes
🐘 PG `CREATE DATABASE ... TEMPLATE`
🐬 MySQL schema replication
🪶 SQLite APFS/CoW
```

---

### Tweet 4 (Features)
```text
What's inside BranchBase v0.3.0:

🐳 Auto-detects Docker Compose files and port bindings
🖥️ Built-in ANSI Terminal UI dashboard (`branchbase tui`)
🧹 `branchbase prune` for cleaning merged databases
🛡️ TLS/SSL client negotiation & UNIX domain socket support
```

---

### Tweet 5 (Call to Action)
```text
Ready to try it?

📦 Install via Homebrew:
`brew install oscarbol09/branchbase/branchbase`

⭐ Star on GitHub: https://github.com/oscarbol09/branchbase

Built with Go 🐹. Feedback and PRs welcome!

#OpenSource #Golang #PostgreSQL #DevTools #Docker
```
