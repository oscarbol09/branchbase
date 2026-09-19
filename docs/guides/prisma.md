# Using BranchBase with Prisma ORM

This guide demonstrates how to use **BranchBase** with **Prisma ORM** (TypeScript / Node.js) for instant, conflict-free local database branching.

---

## Problem: Prisma Migration Conflicts in Git

When collaborating on full-stack apps with Prisma, switching branches often breaks local development:
1. You run `npx prisma migrate dev` on `feature/stripe-billing`, adding new tables and non-nullable columns.
2. An urgent bug comes in on `main` -> you run `git checkout main`.
3. Starting your backend server crashes with:
   ```text
   PrismaClientKnownRequestError: The column `users.stripe_customer_id` does not exist in the current database.
   ```
4. Resetting the database (`npx prisma migrate reset`) wipes all your mock seeds and test logins.

---

## Solution: BranchBase + Prisma

**BranchBase** transparently routes Prisma queries to an isolated, copy-on-write clone of your development database whenever you switch Git branches. Your application's `DATABASE_URL` **never changes**.

---

## Step-by-Step Setup

### 1. Initialize BranchBase in your Prisma Project

In your project repository:
```bash
branchbase init
```
This generates `.branchbase.json` and installs automated Git hooks in `.git/hooks/`.

### 2. Configure your `.env`

Point Prisma to the BranchBase transparent proxy (default port: `5432`):

```env
# .env
DATABASE_URL="postgresql://postgres:postgres@localhost:5432/myapp_dev?schema=public"
```

### 3. Start the BranchBase Proxy

```bash
branchbase proxy
```

---

## Daily Git Workflow

### 1. Create and work on a feature branch
```bash
git checkout -b feature/stripe-billing
```
BranchBase's Git hook instantly snapshots your `myapp_dev` database into `myapp_dev_feature_stripe_billing`.

### 2. Run Prisma migrations freely
```bash
npx prisma migrate dev --name add_stripe_billing
```
Your migrations apply solely to the isolated `myapp_dev_feature_stripe_billing` database.

### 3. Switch back to `main` anytime
```bash
git checkout main
```
BranchBase immediately routes all Prisma queries back to your clean `myapp_dev` database. Zero schema mismatch errors!

### 4. Check active branch database status
```bash
branchbase status
```
Output:
```text
BranchBase Status
  • Active Git Branch: main
  • Target Database:   myapp_dev
  • Driver:            postgres
  • Proxy Port:        5432 -> Backend: 127.0.0.1:5433
```

---

## Best Practices

1. **Client Generation:** Run `npx prisma generate` after branch switches if schema types changed.
2. **Pruning Merged Databases:** After your pull request is merged into `main`, clean up disk space with:
   ```bash
   branchbase prune
   ```
