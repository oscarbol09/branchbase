# Quickstart Guide

Get BranchBase configured and running with your application in under 60 seconds.

---

## 1. Initialize your Repository

Navigate to your project root where your Git repository and database configuration reside:

```bash
cd my-awesome-project
branchbase init
```

`branchbase init` performs the following automated steps:
1. Detects your Git workspace and active branch.
2. Auto-inspects `docker-compose.yml` or local database configurations (PostgreSQL, MySQL, SQLite).
3. Generates a lightweight `.branchbase.json` configuration file.
4. Installs automated `post-checkout` and `post-merge` Git hooks into `.git/hooks/`.

---

## 2. Start the Transparent Proxy

Start the routing proxy on your default database port:

```bash
branchbase proxy
```

Your backend application continues querying `localhost:5432` as usual:
```env
# .env (NEVER NEEDS EDITING AGAIN)
DATABASE_URL="postgres://postgres:postgres@localhost:5432/myapp_dev"
```

---

## 3. Work with Git

Switch branches and run schema migrations freely:

```bash
# 1. Create and switch to a feature branch
git checkout -b feature/stripe-billing

# 2. Run your ORM migrations freely
npx prisma migrate dev
# or: rails db:migrate / alembic upgrade head / go run ./migrations

# 3. Insert mock data, test new tables...
```

When you need to switch back to `main` to test a bug or review a PR:

```bash
git checkout main
```

BranchBase immediately points your proxy to the clean `main` database without migration conflicts or lost test data.

---

## 4. Explore the Interactive Dashboard

Launch the built-in terminal UI to view all branch databases, inspect sizes, and switch contexts with arrow keys:

```bash
branchbase tui
```

Use `↑`/`↓` to navigate, `Enter` to switch branches, and `q` to exit.
