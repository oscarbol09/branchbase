# Hacker News Launch Guide: "Show HN" 🚀

> **Target Platform:** [news.ycombinator.com](https://news.ycombinator.com/)  
> **Best Timing:** Tuesday or Wednesday between 08:00 AM – 10:00 AM ET (Eastern Time).  
> **Format:** Link post submission + immediate first comment by author.

---

## 1. Submission Details

* **Title:**  
  `Show HN: BranchBase – Git-native database branching for PostgreSQL, MySQL, and SQLite`
* **URL:**  
  `https://github.com/oscarbol09/branchbase`

---

## 2. Author's First Comment (Post immediately after submitting)

```text
Hi HN! I built BranchBase because I was tired of the "git checkout main -> 💥 migration error" loop during local development.

The problem:
When you're building a feature on `feature/stripe-billing`, you run database migrations (new tables, strict foreign keys, NOT NULL columns). Then an urgent bug alert arrives on `main`. You run `git checkout main`, start your dev server or test suite, and everything crashes with ActiveRecord::PendingMigrationError or Prisma column mismatch errors.

The usual workarounds are painful:
1. Running `docker compose down -v && docker compose up -d` (losing mock data and seeds).
2. Manually rolling back migrations before switching (tedious and risky).
3. Maintaining multiple connection strings in `.env` and manually switching ports.
4. Cloud branching services (Neon, PlanetScale) which are great, but proprietary, paid, and require an internet connection.

How BranchBase solves this:
BranchBase runs 100% locally on your machine and uses Git hooks (`post-checkout` and `post-merge`) to detect branch switches:
- PostgreSQL: Creates instant branch clones in milliseconds using `CREATE DATABASE ... TEMPLATE`.
- MySQL / MariaDB: Clones schemas and tables with transactional replication.
- SQLite: Uses filesystem Copy-on-Write snapshots (APFS `clonefile(2)` on macOS, `ioctl(FICLONE)` on Linux Btrfs/XFS, fast stream fallback on Windows).
- Transparent Wire Proxy: Intercepts standard port 5432, parses the PostgreSQL `StartupMessage` packet, rewrites the target database to your active branch, and proxies traffic with zero latency. Your app's DATABASE_URL never changes.
- Automatic Pruning: `branchbase prune` removes merged ephemeral databases so you don't accumulate disk space.

It also includes Docker Compose auto-detection and an interactive ANSI Terminal UI (`branchbase tui`).

BranchBase is 100% open-source (MIT), written in Go with minimal dependencies, and works completely offline.

GitHub: https://github.com/oscarbol09/branchbase

I would love to get your feedback on the architecture, especially the wire-protocol rewriting approach for local connection proxies. I'm happy to answer any questions!
```

---

## 3. Hacker News Interaction Strategy

1. **Be Active for the first 4 hours:** Check replies every 10–15 minutes.
2. **Humility & Engineering Focus:** Never argue or use marketing buzzwords. If someone raises a technical limitation (e.g. large DB template clone times), explain how BranchBase handles it (pre-warming, connection pool draining) and thank them for the suggestion.
3. **No vote manipulation:** Never ask friends or colleagues to upvote directly on HN (their anti-spam algorithm detects referral rings and flags posts). Share the link naturally on Twitter/socials asking for feedback.
