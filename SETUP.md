# Development Environment Setup 💻

This guide walks you through setting up your local machine to build, test, and contribute to **BranchBase**.

---

## 1. Prerequisites

Make sure you have the following installed on your workstation:

- **Go (1.22+):** [Download and install Go](https://go.dev/dl/). Verify with `go version`.
- **Git (2.30+):** Verify with `git --version`.
- **Docker & Docker Compose:** Required to run local test database containers.
- **pre-commit (Optional but recommended):** `pip install pre-commit` or `brew install pre-commit`.

---

## 2. Clone & Initial Build

```bash
# Clone your fork
git clone https://github.com/<your-username>/branchbase.git
cd branchbase

# Install pre-commit hooks
pre-commit install

# Verify build
go build -v -o bin/branchbase ./cmd/branchbase

# Test running the CLI binary
./bin/branchbase version
```

---

## 3. Running Test Database Containers

To test database branching against real PostgreSQL instances locally:

```bash
# Start a local PostgreSQL 16 test container
docker run --name branchbase-postgres-test \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=myapp_dev \
  -p 5433:5432 \
  -d postgres:16-alpine

# Verify connectivity
docker exec -it branchbase-postgres-test pg_isready
```

---

## 4. Running the Test Suite

```bash
# Run all unit tests
go test -v ./...

# Run tests with data race detector
go test -v -race ./...

# Run test coverage calculation
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

---

## 5. Testing the CLI Manually

```bash
# 1. Initialize configuration in a test folder
mkdir test-workspace && cd test-workspace
git init
../bin/branchbase init

# 2. Inspect status
../bin/branchbase status

# 3. Test starting the transparent proxy
../bin/branchbase proxy
```

---

## 6. Troubleshooting Common Issues

### Issue: `bind: address already in use (5432)`
Another local database instance (like native Postgres or another container) is occupying port 5432.
- **Fix:** In `.branchbase.json`, change `proxy.listen_port` to an unused port (e.g. `54320`), or stop the conflicting service (`sudo service postgresql stop` or `docker stop <container>`).

### Issue: `cannot read .git/HEAD`
BranchBase must be run inside a Git repository.
- **Fix:** Ensure you are in a directory where `git rev-parse --is-inside-work-tree` returns `true`.
