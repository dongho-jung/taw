# CI/CD

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         GitHub Actions                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Push to main / PR          Tag push (v*)                       │
│        │                          │                             │
│        ▼                          ▼                             │
│  ┌──────────┐              ┌─────────────┐                      │
│  │    CI    │              │   Release   │                      │
│  └──────────┘              └─────────────┘                      │
│        │                          │                             │
│        ├── Lint                   └── GoReleaser                │
│        ├── Test                         │                       │
│        └── Build                        ├── Build (6 platforms) │
│              │                          ├── Archive             │
│              └── Artifacts              ├── Checksum            │
│                  (7 days)               └── GitHub Release      │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## CI Workflow

**File**: `.github/workflows/ci.yml`
**Trigger**: Push to `main` branch or PR

### Jobs

| Job | Description | Details |
|-----|-------------|---------|
| Lint | Code quality check | Run `golangci-lint` |
| Test | Run tests | `go test -v -race ./...` |
| Build | Multi-platform build | Generate binaries for 5 platforms |

### Build Matrix

| OS | Arch | Filename |
|----|------|----------|
| linux | amd64 | `paw-linux-amd64` |
| linux | arm64 | `paw-linux-arm64` |
| darwin | amd64 | `paw-darwin-amd64` |
| darwin | arm64 | `paw-darwin-arm64` |
| windows | amd64 | `paw-windows-amd64.exe` |

Build artifacts are retained for 7 days.

## Release Workflow

**File**: `.github/workflows/release.yml`
**Trigger**: Push tag `v*` (e.g., `v1.0.0`, `v2.1.0-beta`)

### Release Process

1. GoReleaser builds binaries for 6 platforms
2. Create archives (tar.gz, zip for Windows)
3. Generate SHA256 checksums
4. Auto-create GitHub Release

### GoReleaser Configuration

**File**: `.goreleaser.yaml`

**Build Options**:
- `CGO_ENABLED=0` (static binary)
- ldflags: `-s -w` (strip debug symbols)
- Version info injection: `Version`, `Commit`, `Date`

**Archive Format**:
| OS | Format | Example |
|----|--------|---------|
| linux/darwin | tar.gz | `paw_1.0.0_darwin_arm64.tar.gz` |
| windows | zip | `paw_1.0.0_windows_amd64.zip` |

**Changelog**:
- Auto-grouping based on Conventional Commits
- `feat:` → Features
- `fix:` → Bug Fixes
- `perf:` → Performance
- `refactor:` → Refactor
- `docs:`, `test:`, `chore:`, `ci:` → Excluded

## How to Release a New Version

```bash
# 1. Create version tag
git tag v1.0.0

# 2. Push tag (Release workflow runs automatically)
git push origin v1.0.0
```

Once released, you can verify it on the GitHub Releases page.

## Local Build

```bash
# Regular build
make build

# Local install (~/.local/bin)
make install

# Global install (/usr/local/bin, requires sudo)
make install-global

# Test
make test

# Lint
make lint
```

## Secrets

| Secret | Purpose | Required |
|--------|---------|----------|
| `GITHUB_TOKEN` | GoReleaser release creation | Auto-provided (no setup needed) |
