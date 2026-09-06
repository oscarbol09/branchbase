# Changelog

All notable changes to **BranchBase** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- Transparent TCP proxy server with dynamic Git branch routing.
- PostgreSQL driver skeleton utilizing `CREATE DATABASE ... TEMPLATE`.
- Git HEAD branch resolver and SQL identifier sanitization engine.
- Configuration manager supporting `.branchbase.json` and `.branchbase.yaml`.
- CLI commands: `init`, `status`, `proxy`, `list`, `prune`, `version`.
- Architecture specification (`ARCHITECTURE.md`) with Mermaid sequence diagrams.
- Contributing guidelines (`CONTRIBUTING.md`), Code of Conduct (`CODE_OF_CONDUCT.md`), and Security Policy (`SECURITY.md`).
- Multi-OS GitHub Actions CI workflow (Ubuntu, macOS, Windows).
- Pre-commit configuration and EditorConfig standardization.
- BranchBase driver skill (`.agents/skills/branchbase-driver/SKILL.md`).

---

## [0.1.0-alpha] - 2026-09-06

### Inception
- Initial project scaffolding and public repository launch.
