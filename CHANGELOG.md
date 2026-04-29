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
- Activities can now be flagged as "watering" and/or "feeding". The built-in "Water" and "Feed" activities are migrated to set the matching flag, and the dashboard "days since last watering / feeding" calculations now use the flags rather than hardcoded names — so renaming or translating the built-in activities no longer breaks tracking. (PR #160)
- Extensive Testing overhaul, dramatically improved test coverage
- New `ISLEY_SECURE_COOKIES` documented in the README env-var table — set to `true` when fronting Isley with a TLS reverse proxy.
- `dependabot.yml` now also tracks `docker` and `github-actions` ecosystems so base-image and action bumps surface as PRs instead of stale pins.
- New `ISLEY_HSTS_MAX_AGE`, `ISLEY_HSTS_INCLUDE_SUBDOMAINS`, and `ISLEY_HSTS_PRELOAD` environment variables documented in the README — opt-in `Strict-Transport-Security` for HTTPS-only deployments.
- `strain_lineage` now travels through the SQLite→PostgreSQL bootstrap migration tool (`model/sqlite_to_postgres.go`), so users migrating an existing SQLite install with lineage data no longer silently lose it on the Postgres side.

### Changed
- The activities page now matches the look and feel of the plants, strains, and sensors pages: in-page search, filter dropdowns, sortable column headers, and a result count, with no full-page reload between filter changes. CSV and XLSX export honor the active filters. Behind the scenes, the `/activities/list` JSON endpoint now accepts a larger `page_size` so the page can render the full activity log client-side. Shared CSS for the controls bar and table styling across the four list views was deduplicated. (PR #159)
- `/health` now pings the database with a 2-second timeout and returns 503 when the DB is unreachable, so the Dockerfile HEALTHCHECK no longer reports a container with an offline backend as healthy.
- The shipped `docker-compose.{sqlite,postgres,migration}.yml` files now resolve the Isley image as `dwot/isley:${ISLEY_VERSION:-latest}` so deployments can pin a specific release without editing the compose file.
- `ISLEY_DB_SSLMODE` README description corrected to show the actual `disable` default and the recommended values for TLS-enforcing Postgres hosts.
- AC Infinity / EcoWitt scan handlers now surface upstream network and parsing failures as `502 Bad Gateway` (with localized error keys) instead of silently returning `200 OK`.
- The `/api/overlay` endpoint now shares the ingest rate limiter (60 req/min/key) so an unauthenticated misconfiguration can no longer hammer it unbounded.
- Bumped Linux release-binary Go toolchain in `.github/workflows/release.yml` from `1.25.0` to `1.25.8`, matching the test jobs.
- GitLab CI `sign-dev` and `publish_dev_to_github` jobs upgraded from `alpine:3.20` to `alpine:3.23` to match the Dockerfile runtime base.
- `RecordMultiPlantActivity` now wraps its N inserts in a single transaction with a prepared statement — partial failures no longer leave the activity log inconsistent and SQLite pays one WAL fsync instead of N.
- AC Infinity / EcoWitt scan handlers now route their sensor registration through the locked `findOrCreateSensor` helper, eliminating an unprotected SELECT-then-INSERT path that could create duplicate rows under concurrent scans.

### Deprecated

### Removed
- Removed the per-service `healthcheck:` blocks from the shipped
  `docker-compose.*.yml` files for the `isley` service. Compose-level
  healthchecks fully override the Dockerfile `HEALTHCHECK`, so the
  hardcoded `localhost:8080` probe ignored `ISLEY_PORT` even after the
  v0.1.42 Dockerfile fix. The Dockerfile healthcheck (which honors
  `${ISLEY_PORT:-8080}`) is now the single source of truth.
- Deleted the unused `jsonify` template helper (returned unescaped JSON; previous audits flagged it as a footgun) and the duplicate `formatDateISO` template func — `formatDate` already produced an identical ISO date.
- Removed two redundant placeholder/args slice loops in `getPlantsByStatus` whose results were immediately discarded by a re-declaration on the next line.
- Dropped the unused `ISLEY_MIGRATE_SQLITE` line from `docker-compose.migration.yml` — the variable was never read by Go code (the migration scan uses `ISLEY_DB_FILE`).

### Fixed
- Reopened #146: shipped Compose files were overriding the Dockerfile
  healthcheck and pinning the probe to port 8080. Users who changed
  `ISLEY_PORT` saw the container marked unhealthy. Resolved by removing
  the redundant Compose healthcheck blocks; existing deployments may
  need to `docker compose up -d --force-recreate` (or edit their local
  compose file) to pick up the new behavior.
- Sensor ingest endpoint no longer creates duplicate sensor rows under concurrent ingest of the same (source, device, type). Existing duplicates are merged on upgrade.

### Security
- Plant image uploads are now bounded with `http.MaxBytesReader` (250 MB per request, 50 MB per file) and rejected with `413 Request Entity Too Large` when exceeded — previously, files above the in-memory threshold spilled to disk unbounded.
- `LinkSensorsToPlant` now verifies every supplied sensor ID exists before persisting the JSON column, rejecting dangling references with `400` instead of silently storing them.
- Strain create/update endpoints now length-validate `name`, `description`, `short_desc`, and `new_breeder`, and reject `url` values that are not http/https — closes a long-standing input-validation gap on the strain form.
- Plant activity, measurement, and status edit endpoints now cap `note` at `MaxNotesLength` (5000) and reject malformed `date` values via the new `utils.ValidateDate` helper.
- Rate-limit logs no longer record raw `X-API-KEY` values; offending callers are now identified by a non-reversible 12-char SHA-256 prefix, preserving correlation while removing the credential from logs.
- `app.NewEngine` now requires `SessionSecret` to be at least 32 bytes (was: any non-empty length), preventing weak session keys.
- Trusted-proxy list now includes the IPv6 unique-local block (`fc00::/7`), so deployments that front Isley with an IPv6 reverse proxy see accurate `c.ClientIP()` values for rate limiting and audit logs.
- CSP-nonce generation now aborts the request with `500 Internal Server Error` on `crypto/rand.Read` failure instead of serving a page with a predictable empty nonce.
- Bumped bcrypt cost from 10 to 12 for password and API-key hashing. New env var `ISLEY_BCRYPT_COST` lets test suites lower the cost without weakening production defaults.
- Bounded watcher and scan response bodies with `io.LimitReader` (4 MiB cap) so a slow-drip server (notably user-supplied EcoWitt LAN addresses) cannot stream unbounded payloads inside the request timeout.
- Capped the `sensor_data_hourly` rollup queries with the same `MaxRawDataRows` limit the raw-data branches already used (defensive only).

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
