# Product Hunt Launch Kit 🚀

> **Target Platform:** [Product Hunt](https://www.producthunt.com/)  
> **Category:** Developer Tools, Open Source, Databases

---

## 1. Listing Metadata

* **Product Name:**  
  `BranchBase`
* **Tagline:**  
  `Git-native local database branching for PostgreSQL & MySQL`
* **Topics:**  
  `Developer Tools`, `Open Source`, `Databases`, `GitHub`, `Productivity`
* **Website / Link:**  
  `https://github.com/oscarbol09/branchbase`

---

## 2. Description (Up to 260 characters)

```text
Zero-config, Git-native local database branching for PostgreSQL, MySQL, and SQLite. Stop dropping or rebuilding your local database every time you switch Git branches. 100% local, offline, and open-source.
```

---

## 3. Maker's First Comment (Post immediately on launch)

```text
Hey Product Hunt community! 👋

I'm Oscar, the creator of BranchBase 🌿.

Every backend developer knows the frustration: you're working on a feature branch with new database migrations. An urgent bug comes in on `main`, so you run `git checkout main`. You launch the app, and boom 💥 — migration mismatch errors crash your server.

The existing solutions are either painful (`docker compose down -v` losing all your test data) or locked behind expensive cloud subscriptions (Neon, PlanetScale).

I built BranchBase to make database branching seamless, instant, and 100% local:
✨ Auto-detects `git checkout` via hooks in < 5ms
✨ Creates instant zero-copy branch snapshots (PostgreSQL TEMPLATE, MySQL table replication, SQLite APFS CoW)
✨ Transparent proxy intercepts port 5432 so your app's DATABASE_URL never changes
✨ Pure-Go interactive Terminal Dashboard (`branchbase tui`)
✨ Auto-detects Docker Compose setups on `branchbase init`
✨ MIT Licensed, zero telemetry, works completely offline

Check out the repo at https://github.com/oscarbol09/branchbase — I'd love to hear your thoughts and feedback!
```
