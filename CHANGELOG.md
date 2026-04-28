# Changelog

All notable changes to Isley are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Maintainer notes

- Edit the `[Unreleased]` section as part of each dev-branch commit that lands
  a user-visible change. One-liners are fine; the idea is "describe it while
  the context is fresh," not "write perfect prose."
- When cutting a release, run `scripts/cut-release.sh X.Y.Z` on a
  `release/vX.Y.Z` branch. The script renames `[Unreleased]` to
  `[X.Y.Z] - YYYY-MM-DD` and seeds a fresh empty `[Unreleased]` block.
- The GitHub release workflow extracts the matching version's section from
  this file and uses it as the release body. Keep the headings exact.
- Historical releases prior to `0.1.42` are catalogued on GitHub Releases;
  they predate this file and were generated from commit logs.

## [Unreleased]

### Added
- Fixed an ordering bug when uploading multiple plant images at once
  where descriptions and dates could be paired with the wrong image. (PR #157)
- The plant detail page now has prev/next arrows in the hero and responds to ArrowLeft / ArrowRight keyboard navigation, cycling within the plant's current top-level state (Living / Harvested / Dead). (PR #158)

### Changed

### Deprecated

### Removed
- Removed the per-service `healthcheck:` blocks from the shipped
  `docker-compose.*.yml` files for the `isley` service. Compose-level
  healthchecks fully override the Dockerfile `HEALTHCHECK`, so the
  hardcoded `localhost:8080` probe ignored `ISLEY_PORT` even after the
  v0.1.42 Dockerfile fix. The Dockerfile healthcheck (which honors
  `${ISLEY_PORT:-8080}`) is now the single source of truth.

### Fixed
- Reopened #146: shipped Compose files were overriding the Dockerfile
  healthcheck and pinning the probe to port 8080. Users who changed
  `ISLEY_PORT` saw the container marked unhealthy. Resolved by removing
  the redundant Compose healthcheck blocks; existing deployments may
  need to `docker compose up -d --force-recreate` (or edit their local
  compose file) to pick up the new behavior.
- Sensor ingest endpoint no longer creates duplicate sensor rows under concurrent ingest of the same (source, device, type). Existing duplicates are merged on upgrade.

### Security

## [0.1.42] - 2026-04-25

### Added
- Added #148 Activities list full view and export.  Activity log is now a top level item with searching / filtering /
export.  Additionally the plant details page includes an expandable "Notebook" view with activity details displayed.

### Changed
- Rebuilt the CI/CD pipeline around a CHANGELOG-driven release ceremony.
  Release notes now come from the curated `CHANGELOG.md` section matching
  `VERSION` rather than from parsed commit messages, which were structurally
  lossy after the GitLab → GitHub squash sync.
- GitHub workflow adds OCI image labels (revision, source, version, created)
  so published Docker Hub images are traceable back to the commit that built
  them.
- GitLab → GitHub squash-sync commit message now embeds the originating
  GitLab short SHA for cross-registry traceability.
- Tightened `VERSION` discipline: release workflow rejects a push to `main`
  if the tag already exists or the version string isn't strict SemVer.

### Deprecated

### Removed

### Fixed
- Fixed #146 Healthcheck didn't respect ISLEY_PORT

### Security
Bump golang to 1.25.9 for security updates
Bump modernc.org/sqlite to 1.49.1
Bump golang.org/x/text to 0.36.0
Bump golang.org/x/image to 0.39.0
Bump golang.org/x/crypto to 0.50.0

## 0.1.41b and earlier

Prior releases are recorded on
[GitHub Releases](https://github.com/dwot/isley/releases). Beginning with
`0.1.42` all user-visible changes are tracked here.
