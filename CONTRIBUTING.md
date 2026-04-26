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
