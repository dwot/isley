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

### Security

## 0.1.41b and earlier

Prior releases are recorded on
[GitHub Releases](https://github.com/dwot/isley/releases). Beginning with
`0.1.42` all user-visible changes are tracked here.
