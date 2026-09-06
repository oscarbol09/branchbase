---
name: "Driver Proposal / RFC"
about: Propose a new database engine driver or architecture RFC for BranchBase
title: "[RFC]: Support for <Database Engine>"
labels: ["rfc", "driver", "needs-discussion"]
assignees: ""
---

## Problem Statement

Which database engine are you proposing to support, and what is the typical local developer setup (Docker, local daemon, embedded)?

## Proposed Branching Mechanism

Explain how this database can achieve fast, copy-on-write or low-latency branching:
- [ ] Native template / cloning command (e.g. Postgres `CREATE DATABASE ... TEMPLATE`)
- [ ] Filesystem-level CoW / Reflink (e.g. SQLite with APFS / Btrfs / XFS)
- [ ] Container volume snapshot / LVM / ZFS
- [ ] Logical schema dump & restore

## Proxy & Wire Protocol Considerations

Does the transparent proxy need to rewrite startup packets or handle specific wire-protocol handshakes for this engine?

## Acceptance Criteria & Edge Cases

What edge cases should be considered (e.g., active open connections, WAL locks, multi-tenant databases)?
