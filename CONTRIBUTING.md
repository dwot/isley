# Contributing to Isley

Thanks for your interest in contributing! This guide covers everything you need to get a local development environment running, build the project, run tests, and understand how the codebase is organized.

## Prerequisites

- **Go 1.25.0** — install from [golang.org](https://golang.org/dl/) or use a version manager like `goenv`.
- **Docker & docker-compose** — needed to run integration tests and for the containerized workflow.
- **FFmpeg** — required at runtime for webcam snapshot capture (optional for most development work).
- **PostgreSQL 16** (optional) — only needed if you're testing against PostgreSQL locally. SQLite works out of the box with no extra dependencies.

## Local Development Setup

1. **Clone the repository:**

   ```bash
   git clone https://github.com/dwot/isley.git
   cd isley
   ```

2. **Install Go dependencies:**

   ```bash
   go mod download
   ```

3. **Run with SQLite (simplest):**

   ```bash
   export ISLEY_DB_DRIVER=sqlite
   export GIN_MODE=debug
   go run .
   ```

   The app starts on `http://localhost:8080`. Default login is `admin` / `isley`.

4. **Run with PostgreSQL:**

   ```bash
   export ISLEY_DB_DRIVER=postgres
   export ISLEY_DB_HOST=localhost
   export ISLEY_DB_PORT=5432
   export ISLEY_DB_USER=isley
   export ISLEY_DB_PASSWORD=supersecret
   export ISLEY_DB_NAME=isleydb
   export GIN_MODE=debug
   go run .
   ```

## Building

```bash
go build -o isley .
```

The Docker image uses a two-stage Alpine build. To build it locally:

```bash
docker build -t isley:dev .
```

## Running Tests

Integration tests live in `tests/integration/` and require a running database.

```bash
# Run all tests
go test ./...

# Run integration tests only
go test ./tests/integration/ -v
```

The integration test suite handles its own database setup and teardown.

### Test helpers

Shared test fixtures live under `tests/testutil/` (and `tests/testutil/fakes/`),
with response-body and archive fixtures under `tests/fixtures/`. Helpers used
in more than one test file MUST live in `tests/testutil` (or a sub-package).
File-local helpers may exist for one-off needs but should not be copied
between files. Reviewers enforce this in PRs.

When a fixture composes existing testutil primitives, prefer that over raw
SQL — even when the raw SQL is shorter. The point of the primitives is that
one place owns the column ordering and default values; bypassing them in a
fixture function silently re-introduces the drift the consolidation was
meant to prevent.

#### Authenticating mutating requests in tests

Two patterns coexist, and they exercise different production code paths:

- **`X-API-KEY` (`Client.APIPostJSON`, `Client.APIDelete`).** Use this for
  the ingest contract (sensor data, operator scripts, external
  integrations). Setting `X-API-KEY` causes `csrfMiddleware` to skip
  validation entirely, which is correct for that contract but means
  these tests do **not** cover the CSRF round-trip.
- **Session cookie + CSRF (`TestServer.LoginAndFetchCSRF` →
  `Client.SessionPostJSON`).** Use this for any handler whose real
  user-facing contract is the dashboard — i.e. the same path browser JS
  takes. `LoginAndFetchCSRF` logs in via `LoginAsAdmin` and then GETs
  the supplied edit page to extract the CSRF token from
  `<meta name="csrf-token">`; `SessionPostJSON` forwards the token in
  `X-CSRF-Token`. This is the standard for new session-path tests.

`tests/integration/session_csrf_test.go` keeps the cookie + CSRF
round-trip covered for the major resources (plant, strain, settings,
sensor edit, zone). When you add a new dashboard-only mutating
endpoint, add a single round-trip test next to those.

### Coverage floor

CI gates every PR on a documented coverage floor — one per build tag. The
SQLite floor lives in `.github/workflows/release.yml` and `.gitlab-ci.yml`
as `floor=11.0`; the `integration_postgres` floor lives in the same files
as `floor=2.0`. A PR that drops total statement coverage below either
floor fails the pipeline.

The numbers are intentionally low: CI runs `go test -coverpkg=./...`,
which credits coverage across the whole tree (untested packages
drag the total down) rather than the per-package average `go test
-cover` reports by default. The two are not comparable — the
per-package number for this codebase is roughly 5× larger. See
`audits/TESTING.md`'s "Phase 6c recalibration" note for why the
cross-tree number is the honest one. If you check coverage locally,
run with `-coverpkg=./...` so your reading matches CI's:

```bash
go test -race -coverpkg=./... -coverprofile=coverage.out -timeout=20m ./...
go tool cover -func=coverage.out | tail -1
```

Bumping rules:

- **PRs that increase coverage MAY bump the floor accordingly.** If your
  change ratchets the SQLite total from 12.4% to 14.0%, you may bump
  `floor=11.0` → `floor=13.0` in the same PR. Use `floor(total - 1.0)`
  rounded down — a 1.0-point slack absorbs harmless churn from later
  refactors that don't touch the test surface.
- **PRs that don't change coverage MUST NOT bump the floor down.** The
  floor is a ratchet. If a refactor moves coverage from 12.4% to 11.4%
  without removing any tests, fix the test (or the refactor) — don't
  loosen the gate.
- **Raising the floor is a deliberate choice.** The default is to let
  coverage rise without raising the floor. Raise it only when you want
  the new number to become the contract.

The Phase 7 (`docs/TEST_PLAN_2.md`) work that lifts dialect-branched
handler tests into the `integration_postgres` job will produce a real
Postgres coverage number; that's the right time to recalibrate the
Postgres floor from its current placeholder of 2.0%.

## Project Structure

```
isley/
├── main.go                  # Entry point, router setup, middleware
├── config/                  # Runtime configuration (loaded from DB settings)
├── handlers/                # HTTP handler functions
│   ├── plant.go             # Plant CRUD and detail loading
│   ├── overlay.go           # Live-stream overlay API
│   ├── sensors.go           # Sensor management and scanning
│   ├── sensor_data.go       # Sensor data charting endpoint
│   ├── settings.go          # Application settings
│   ├── db.go                # Database context helper (DBFromContext)
│   ├── api_errors.go        # Shared API error responses
│   └── ratelimit.go         # Rate limiting middleware
├── model/
│   ├── migrate.go           # Database initialization and migrations
│   ├── sqlite_to_postgres.go # SQLite → PostgreSQL data migration
│   ├── migrations/
│   │   ├── sqlite/          # SQLite migration SQL files
│   │   └── postgres/        # PostgreSQL migration SQL files
│   └── types/
│       └── base_models.go   # Shared Go structs (Plant, Sensor, etc.)
├── routes/
│   └── routes.go            # Route group definitions
├── utils/
│   ├── locales/             # YAML translation files (en, de, es, fr)
│   └── ...                  # Validation, image processing, i18n helpers
├── watcher/                 # Background sensor polling loop
├── web/
│   ├── templates/           # Go HTML templates
│   │   ├── common/          # Header, footer, shared modals
│   │   └── views/           # Page templates (index, plant, sensors, etc.)
│   └── static/
│       ├── css/             # Stylesheets (isley.css)
│       └── js/              # Client-side JavaScript
├── tests/
│   └── integration/         # Integration test suite
├── Dockerfile               # Multi-stage Alpine build
├── docker-compose.*.yml     # Compose files for SQLite, PostgreSQL, migration
└── VERSION                  # Current release version
```

## Key Patterns

**Database access:** The database middleware injects `*sql.DB` into every Gin request context. Handlers should use `handlers.DBFromContext(c)` to retrieve it. Standalone utility functions that don't have a Gin context use `model.GetDB()` directly.

**Dual database support:** The app supports both SQLite and PostgreSQL. SQL queries use `$1` placeholder style (which works for both). Where dialect-specific SQL is needed, branch on `model.IsPostgres()`. Place new migration files in both `model/migrations/sqlite/` and `model/migrations/postgres/`.

**Templates:** Gin loads Go HTML templates from `web/templates/`. Localized strings come from YAML files in `utils/locales/` and are passed to templates as `.lcl`.

**Environment variables:** Application behavior is configured via `ISLEY_*` environment variables (see README). Set `GIN_MODE=debug` during development for verbose logging.

## Submitting Changes

1. Fork the repository and create a feature branch.
2. Make your changes with clear, descriptive commit messages.
3. Run `go vet ./...` and `go test ./...` before opening a PR.
4. Open a pull request against `main` with a description of what changed and why.
