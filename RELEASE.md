# Release Process

This document describes the automated release process for this project using [Semantic Versioning (SemVer)](https://semver.org/).

## Overview

The project uses an automated release workflow that:
1. Analyzes commit messages to determine version bump type
2. Automatically creates git tags
3. Builds cross-platform binaries and Docker images
4. Optionally creates GitHub releases

## Semantic Versioning

We follow SemVer format: `MAJOR.MINOR.PATCH` (e.g., `v1.2.3`)

- **MAJOR**: Breaking changes that are not backward compatible
- **MINOR**: New features that are backward compatible  
- **PATCH**: Bug fixes and small improvements

## Automated Version Bumping

Version bumps are determined by commit message prefixes:

### Major Version Bump (X.0.0)
Use when introducing breaking changes:
```bash
git commit -m "feat: BREAKING CHANGE: redesign API structure"
# OR
git commit -m "major: remove deprecated endpoints"
```

### Minor Version Bump (X.Y.0) 
Use when adding new features:
```bash
git commit -m "feat: add new authentication provider"
# OR  
git commit -m "feature: implement user roles management"
# OR
git commit -m "minor: add configuration validation"
```

### Patch Version Bump (X.Y.Z)
Default for all other commits:
```bash
git commit -m "fix: resolve memory leak in session handler"
git commit -m "docs: update installation instructions"  
git commit -m "refactor: optimize database queries"
```

## Release Process

### Step 1: Create Release Branch
```bash
git checkout main
git pull origin main
git checkout -b release/vX.Y.Z
```

### Step 2: Make Your Changes
Make your code changes and commit with appropriate message prefix:

```bash
# For minor version bump (new features)
git add .
git commit -m "feat: add support for multi-factor authentication"

# For major version bump (breaking changes)  
git add .
git commit -m "BREAKING CHANGE: restructure configuration format"

# For patch version bump (bug fixes, docs, etc.)
git add .
git commit -m "fix: handle edge case in token validation"
```

### Step 3: Create Pull Request
```bash
git push origin release/vX.Y.Z
gh pr create --title "feat: Release vX.Y.Z" --body "Description of changes" --base main
```

### Step 4: Merge PR
Once the PR is reviewed and approved, merge it to main. This triggers:

1. **Auto-tagging**: Creates new version tag based on commit message
2. **Testing**: Runs unit, functional, and integration tests
3. **Building**: Creates cross-platform binaries (Linux, macOS, Windows)
4. **Docker**: Builds and pushes multi-arch Docker images
5. **Artifacts**: Uploads build artifacts

### Step 5: Monitor Build
Check the workflow progress:
```bash
gh run list --workflow="test-and-build.yml"
gh run watch  # Watch the latest run
```

## Manual Release (Optional)

After the automated build completes, you can optionally create a GitHub release:

```bash
gh workflow run manual-release.yml -f tag_version=vX.Y.Z
```

This will:
- Tag the Docker image as `latest`
- Create a GitHub release with changelog
- Attach build artifacts
- Update Homebrew formula (if configured)

## Current Version Strategy

The workflow automatically detects the latest tag and bumps accordingly:

| Current | Commit Message | New Version |
|---------|----------------|-------------|
| v0.0.163 | `feat: add new feature` | v0.1.0 |
| v0.1.5 | `fix: bug fix` | v0.1.6 |
| v1.2.3 | `BREAKING CHANGE: api redesign` | v2.0.0 |

## Docker Images

Released versions are available as:
```bash
docker pull ghcr.io/thand-io/agent:vX.Y.Z
docker pull ghcr.io/thand-io/agent:latest  # Points to latest release
docker pull ghcr.io/thand-io/agent:dev     # Points to latest main build
```

## Binary Downloads

Binaries for each release are available in GitHub releases:
- `agent-linux-amd64.tar.gz`
- `agent-linux-arm64.tar.gz` 
- `agent-darwin-amd64.tar.gz`
- `agent-darwin-arm64.tar.gz`
- `agent-windows-amd64.zip`

## Troubleshooting

### Branch Protection Rules
If direct pushes to main are blocked, always use the PR workflow above.

### Failed Builds
- Check workflow logs: `gh run view <run-id>`
- Ensure tests pass locally before creating PR
- Verify Docker builds work with: `docker build .`

### Missing Artifacts
The manual release workflow requires artifacts from a successful main branch build. If you manually created a tag without going through the PR process, delete the tag and follow the proper workflow:

```bash
git tag -d vX.Y.Z
git push origin --delete vX.Y.Z
# Then follow the PR process above
```

## Examples

### Adding a New Feature (Minor Bump)
```bash
git checkout -b release/v0.2.0
# Make changes
git add .
git commit -m "feat: implement single sign-on integration"
git push origin release/v0.2.0
gh pr create --title "feat: Release v0.2.0" --body "Add SSO support" --base main
# Merge PR → Auto-creates v0.2.0 tag and builds
```

### Bug Fix (Patch Bump)  
```bash
git checkout -b release/v0.1.1
# Fix bug
git add .
git commit -m "fix: prevent session timeout during long operations"
git push origin release/v0.1.1
gh pr create --title "fix: Release v0.1.1" --body "Fix session timeout issue" --base main
# Merge PR → Auto-creates v0.1.1 tag and builds
```

### Breaking Change (Major Bump)
```bash
git checkout -b release/v1.0.0
# Make breaking changes
git add .
git commit -m "BREAKING CHANGE: require authentication for all endpoints"
git push origin release/v1.0.0  
gh pr create --title "Release v1.0.0" --body "Major version with breaking changes" --base main
# Merge PR → Auto-creates v1.0.0 tag and builds
```