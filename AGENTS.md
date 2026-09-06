# Guidelines for AI Agents and Contributors (BranchBase) 🌿

This file defines the project instructions, architectural boundaries, and conventions that all AI agents and human contributors must follow when developing inside this repository.

---

## 1. Commit Conventions (Conventional Commits)

All commit messages in this repository must strictly adhere to the [Conventional Commits v1.0.0](https://www.conventionalcommits.org/) format:

```text
<type>(<optional scope>): <description>

[optional body]

[optional footer(s)]
```

### Allowed Types:
- `feat:` A new user-facing feature or CLI subcommand.
- `fix:` A bug fix in driver, proxy, or resolver.
- `docs:` Documentation only changes (`README.md`, `ARCHITECTURE.md`, etc.).
- `test:` Adding missing tests or correcting existing tests.
- `refactor:` A code change that neither fixes a bug nor adds a feature.
- `perf:` A code change that improves performance (e.g. proxy latency).
- `chore:` Changes to build process, CI workflows, or auxiliary tools.

*Example:* `feat(postgres): implement connection termination before template cloning`

---

## 2. Go Coding Standards & Architecture

1. **Minimal Dependencies:**
   - Prioritize Go standard library (`net`, `io`, `os`, `database/sql`, `regexp`).
   - Do not pull heavy third-party frameworks unless strictly necessary for wire-protocol parsing or CLI UX.
2. **Context Propagation:**
   - Every network and database call must accept `context.Context` as its first argument and respect cancellation/timeouts.
3. **Error Handling:**
   - Never discard errors (`_ = err` is forbidden in business logic).
   - Wrap errors with informative context using `fmt.Errorf("...: %w", err)`.
4. **Test-Driven:**
   - Every new package or driver must include a `_test.go` file.
   - Use table-driven tests for input-output validation.
5. **Cross-Platform Compatibility:**
   - Use `filepath.Join()` for all filesystem paths (avoid hardcoded `/` or `\`).
   - Guard OS-specific syscalls (like `clonefile` or `FICLONE`) using Go build tags (`//go:build darwin`, `//go:build linux`).

---

## 3. Pull Request Standards

- Each PR must solve a single concern.
- The title must follow Conventional Commits.
- Tests must pass before merging.
- If modifying configuration or user commands, update `branchbase.example.yaml` and `README.md`.
