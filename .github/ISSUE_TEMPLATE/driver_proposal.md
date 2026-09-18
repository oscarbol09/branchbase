---
name: Database Driver Proposal
about: Propose and implement support for a new database engine
title: "[DRIVER] Support for <Engine Name>"
labels: ["driver", "enhancement"]
assignees: ''
---

### 🧩 Proposed Database Engine
<!-- Name and version of the database engine (e.g. MongoDB 7.0+, CockroachDB, ClickHouse) -->

### ⚙️ Engine Snapshot / Branching Strategy
<!-- How does this database support fast cloning or snapshotting?
Examples:
- Native clone / snapshot command
- Copy-on-Write volume / filesystem level
- Schema creation + table copy
- Shard/Collection level metadata cloning
-->

### 🔌 Connection & Wire Protocol Requirements
<!-- Does this engine use standard SQL / TCP sockets / specific wire protocol? -->

### 📋 Implementation Checklist
- [ ] Implement `Driver` interface in `internal/driver/<engine>/<engine>.go`
  - [ ] `Name() string`
  - [ ] `Ping(ctx context.Context) error`
  - [ ] `BranchExists(ctx context.Context, branchName string) (bool, error)`
  - [ ] `CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error`
  - [ ] `DeleteBranch(ctx context.Context, branchName string) error`
  - [ ] `ListBranches(ctx context.Context) ([]BranchInfo, error)`
  - [ ] `Close() error`
- [ ] Auto-register factory in `init()` via `driver.Register("<engine>", factory)`
- [ ] Add hermetic unit tests with `_test.go`
- [ ] Add integration test container in CI/CD workflow (`.github/workflows/ci.yml`)
- [ ] Update `README.md` and documentation guides
