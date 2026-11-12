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

- Scoped API keys. Each key now has an access level (read only, read and write,
  or admin) and, separately, permission to push sensor readings. Pick it when
  creating a key in Settings → API, and change it later without changing the
  key. New keys default to read only. A request beyond a key's access gets 403
  with `"code": "insufficient_scope"`.
- The start of API v1 at `/api/v1`: `GET /api/v1/instance` (version, time
  zone, enabled features and the calling key's scopes) and the OpenAPI 3.1
  description at `/api/v1/openapi.yaml`. It takes API keys only, as
  `Authorization: Bearer <key>` or `X-API-KEY`, and every error has the
  shape `{"error": {"code", "message", "details"}}`. More endpoints follow
  resource by resource.
- API v1 zones and lookups. With a read key: `GET /api/v1/zones` (paginated),
  `GET /api/v1/zones/{id}` (the zone with each sensor's latest reading),
  `GET /api/v1/statuses`, `GET /api/v1/metrics` and
  `GET /api/v1/activity-types`, so a client can find the ids that creating
  plants and logging activities need. With an admin key: `POST`, `PATCH` and
  `DELETE /api/v1/zones`. Deleting a zone through the API only works when it
  is empty (`409 zone_not_empty` otherwise); the settings page still deletes
  a zone with everything in it.
- API v1 plants (read key): `GET /api/v1/plants`, filtered by `phase`
  (growing, harvested, dead), `status_id`, `zone_id` or `strain_id`, and
  `GET /api/v1/plants/{id}` with the status history, days per stage, sensors
  (with the VPD target for the current stage) and the latest activities and
  measurements. Age, days since watering and feeding, days in flower and the
  estimated harvest date are worked out by the server.
- API v1 plant writes. With a write key: `POST /api/v1/plants` (with
  `start_date`, the first status, linked sensors and an optional seed-count
  decrement), `PATCH /api/v1/plants/{id}`, `PUT /api/v1/plants/{id}/sensors`,
  `POST /api/v1/plants/{id}/status`, and `PATCH`/`DELETE` on
  `/api/v1/plants/{id}/status-history/{entry_id}`; `GET` on the status
  history with a read key. `DELETE /api/v1/plants/{id}` needs admin.
- API v1 activities and measurements. With a read key: `GET
  /api/v1/activities` (the activity log, filtered by plant, zone, activity
  type, day range and note text), `GET /api/v1/activities/{id}`, and
  `GET /api/v1/plants/{id}/activities` and `/measurements`. With a write
  key: `POST /api/v1/plants/{id}/activities` (with its measurements),
  `POST /api/v1/activities/bulk` (one activity on several plants), `POST
  /api/v1/plants/{id}/measurements`, and `PATCH`/`DELETE` on
  `/api/v1/activities/{id}` and `/api/v1/measurements/{id}`.
- `Idempotency-Key` on API v1 POSTs: a client that retries with the same key
  gets the first response back (`Idempotent-Replayed: true`) instead of
  logging the watering twice. Keys last 24 hours (migration 026).
- API v1 images. With a read key: `GET /api/v1/plants/{id}/images` and
  `GET /api/v1/images/{id}`, each with the file's URL and the day it was
  taken. With a write key: `POST /api/v1/plants/{id}/images` (one JPEG, PNG,
  GIF or WebP per request, as `multipart/form-data`) and
  `DELETE /api/v1/images/{id}`.
- API v1 strains and breeders. With a read key: `GET /api/v1/strains`
  (filter by `breeder_id`, `in_stock`, `q`), `GET /api/v1/strains/{id}` (with
  its parents), `GET /api/v1/strains/{id}/lineage` (the ancestry tree),
  `GET /api/v1/strains/{id}/descendants`, and `GET /api/v1/breeders` and
  `/breeders/{id}`. With a write key: `POST`/`PATCH` on strains and breeders,
  `PUT /api/v1/strains/{id}/lineage`, and
  `POST /api/v1/strains/{id}/seed-count` with a `delta`, which takes seeds
  out (or adds them) without overwriting a change made at the same time.
  Deleting a strain or breeder needs admin.
- API v1 sensors. With a read key: `GET /api/v1/sensors` (filter by
  `zone_id` or `source`) and `GET /api/v1/sensors/{id}`, each with its latest
  reading. With an admin key: `PATCH` (name, zone, unit, visibility) and
  `DELETE /api/v1/sensors/{id}`.
- API v1 sensor history: `GET /api/v1/sensors/{id}/data?from=&to=&bucket=`
  returns raw readings (paged) or one summary per bucket (`5m`, `1h`, `1d`
  ...) with min, average, max and count. Ports also get the share of the
  time they were on and how often they switched on. Hourly and longer
  buckets read the hourly rollup, so long ranges stay fast.
- API v1 streams: `GET /api/v1/streams` (filter by `zone_id`) and
  `GET /api/v1/streams/{id}` with a read key, each with the path of its
  latest frame; `POST`, `PATCH` and `DELETE` need admin (the server fetches
  frames from a stream's URL).
- API v1 smoke reports. With a read key: `GET /api/v1/smoke-criteria`,
  `GET /api/v1/plants/{id}/smoke-reports`, `GET /api/v1/smoke-reports/{id}`
  and `GET /api/v1/strains/{id}/smoke-summary`. With a write key: `POST
  /api/v1/plants/{id}/smoke-reports` (harvested plants only) and `PATCH`/
  `DELETE /api/v1/smoke-reports/{id}`.

- An MCP server at `/mcp` for AI assistants such as Claude, off until turned
  on in Settings → API. It takes API keys as bearer tokens: a read key gets
  the read tools (overview of what's growing, plants, strains, zones, sensor
  history summarised over a range, reference data), a write key also gets
  tools to start plants, log activities, record measurements, change status,
  adjust seed counts and link sensors. No delete tools. Writes are logged
  with the key's name. `GET /api/v1/instance` reports it under
  `features.mcp`. The README shows how to connect Claude Code and Claude
  Desktop.
- An API guide at `/api/v1/docs`, linked from Settings → API: keys and
  access, conventions, dates and times, errors, paging, safe retries, and
  worked examples (log a watering, move a plant to flower, read a night of
  sensor data, ...). The OpenAPI reference stays at `/api/v1/openapi.yaml`.
- API v1 writes are logged on the server with the key's name, the path and
  the result, including writes refused for lack of access.
- `scripts/smoke-api.sh`: a read-only check of an instance's API v1 and MCP
  server, safe to run against a live instance.

- Duplicate breeders (names that differ only in case or surrounding spaces)
  are merged on upgrade (migration 027): each group becomes its oldest
  breeder, which keeps all their strains and a CannaDB link if one of them
  had it. Breeder names are then unique in the database.
### Changed

- **Times are stored in UTC (#141).** On first start, Isley converts the
  times people entered (status changes, activities, measurements) from the
  instance time zone to UTC, once, and records that it has. Pages and the
  API show the same times as before; what changes is that the server's own
  time zone (`TZ`) no longer moves anything. Take a backup before upgrading:
  the conversion is one-way. Postgres sessions now run in UTC whatever the
  server's zone, and a server that stamped times in another zone has those
  converted too. Restoring an older backup converts it the same way.
- API keys are checked once a minute rather than on every request (a
  successful check is remembered briefly). Scripts and agents making many
  calls in a row no longer pay a slow password-style check each time.
  Revoking, regenerating or re-scoping a key, or restoring a backup, still
  takes effect at once. A key's "last used" time now updates at most once a
  minute.
- A plant's start is a calendar day (it could hold a time of day before,
  which nothing showed).
- Plant day counts now agree everywhere and follow the instance time zone
  (#141). The start day is Day 1; days in flower count the same way (the
  first day of flower is day 1, so the number is one higher than before);
  "days since watered/fed" counts calendar days, so a plant watered last
  night shows 1, not 0. The plant timeline shows whole days per stage from
  the server instead of fractions, and its total matches the plant's Day.
- Dates on plant pages are no longer shifted when the timezone setting
  differs from the server's zone.
- The plant page no longer shows "Estimated Harvest Date 0001-01-01" for an
  autoflower whose strain has no cycle time.
- Sensors linked to a plant are stored in a new `plant_sensor` table instead
  of a JSON list on the plant (migration 025). Existing links move over;
  links to sensors that no longer exist are dropped. Backups now include the
  table, and restoring an older backup converts its links the same way.
- Plant and status dates written from the web forms are stored in one
  layout, read in the instance time zone.
- Deleting a plant, or editing or deleting a status entry, that doesn't
  exist now answers 404 instead of reporting success. Deleting a plant also
  clears it as the mother of its clones.
- Creating a plant that takes a seed off its strain now updates the seed
  count shown on the strain pages straight away.
- The activity log's from/to filter now matches by calendar day, so an
  activity stored as a bare date is no longer dropped from a range that
  starts on that day. Its note search ignores case on Postgres as on SQLite.
- Logging an activity now requires the measurements its activity type marks
  as required (the form already asked for them). Deleting an activity or a
  measurement that doesn't exist answers 404 instead of reporting success.
- Uploading several images is all or nothing: if one file is rejected, none
  are saved (before, the ones ahead of it were kept). Uploading to a plant
  that doesn't exist answers 404.
- Deleting an image also deletes its decorated copy, and deleting a plant
  deletes its image files; both used to be left on disk.
- A strain that still has plants can no longer be deleted; the strain page
  says so. (Before, on Postgres the delete failed with a server error, and on
  SQLite the plants were left without a strain.) Strains that named a
  deleted strain as a parent keep the name, unlinked.
- Breeder names are unique, ignoring case. Adding a strain with a "new"
  breeder that already exists uses that breeder instead of making a second
  one. The settings page now shows why adding or renaming a breeder failed.
- A strain can no longer be made its own parent or ancestor. Editing a
  lineage entry that doesn't exist answers 404.
- Editing a sensor and choosing no zone now leaves it in no zone (it used to
  point at a zone that doesn't exist). Sensor edits and deletes show on the
  dashboard straight away instead of after the next cache refresh. Deleting
  a sensor also clears it from any zone's VPD inputs; deleting one that
  doesn't exist answers 404.
- A stream must belong to a zone that exists (Postgres used to fail with a
  server error, SQLite accepted it). Deleting a stream also deletes its
  saved frame; editing or deleting one that doesn't exist answers 404.
- Smoke reports: the report date is a calendar day and cure days count
  calendar days from the harvest day in the instance time zone (they were
  whole 24-hour periods between instants). The add-report form defaults to
  today's local date; in the evening it used to offer tomorrow (the UTC date).
- Settings: editing or deleting a zone, metric or activity that doesn't exist
  now answers 404 instead of reporting success. Validation messages for
  these forms name the field as `name`/`unit` rather than `zone_name` etc.
- Settings: an activity can no longer be renamed to Water, Feed or Note (it
  could already not be created with those names), and a status's VPD range
  is rejected when the low bound is above the high bound.

### Deprecated

### Removed

### Fixed

- A backup limited to the last N days of sensor data used a cutoff in the
  server's local time against readings stamped in UTC, so it kept hours too
  many or too few. The cutoff is now exact.
- API keys' "created" and "last used" times in Settings assumed the database
  stamps in UTC; on a Postgres server in another zone they were off by the
  offset. They now carry their offset.
- Adding or editing a camera stream in Settings failed with "invalid
  payload": the form sent its zone and visibility as text. Both forms work
  again, and editing shows why a save failed. A new stream's first frame is
  saved even when the streams folder doesn't exist yet.
- Sensor data retention on SQLite compared the server's local time with
  readings stored in UTC, so it kept readings for hours too long (servers
  west of UTC) or deleted them hours early (east of UTC). The cutoff is now
  exact.
- A zone's VPD is no longer worked out from a temperature or humidity
  sensor that has stopped reporting (no reading in the last three polls);
  before, it kept recording the same VPD from the last readings forever.
- Sensors that aren't in a zone (ingest creates them when a reading names no
  zone) are listed on the sensors page again; they were silently left out.
- Sensor graphs: a custom date range is read in the instance time zone (it
  was read as UTC), and an end date takes in that whole day. The time
  window for "last N minutes" no longer drifts by the zone offset on a
  Postgres server whose zone isn't UTC. Ranges over a day no longer leave
  out hours the hourly summary missed (after downtime of more than a day,
  or a restore); those hours are read from the raw readings.
- API v1: a POST with an `Idempotency-Key` and a body over 1 MB was cut
  short, and a client that rebuilt the same upload for a retry was refused
  as a different request.

- Turning off API ingest in Settings didn't survive a restart: the setting
  was saved but never read back, so ingest came back on.
- API v1 sensor history labelled 10, 20 and 30 minute buckets as `1`, `2`
  and `3`.

- Pages now show and fill in times in the instance time zone from Settings,
  not the browser's: "now" in the add and edit forms, the times on the plant
  page, the activity log and its date filter, sensor graph labels, API key
  and backup times, and the default day for photos and smoke reports. Seen
  from a browser in another zone, they used to be off by the difference.
- The photo overlay's "Day N" and "Week N" count the start day as day 1,
  like the rest of Isley (it said Day 0, Week 0).
- In guest mode the activity log failed to load, and the plant page logged
  script errors.
- Re-importing a CannaDB breeder no longer renames it to a name another
  breeder already has.

- The "log an activity on several plants" button on a strain's page opened
  with no plants or activities to pick from.

- A CannaDB import is all or nothing: the strain, its breeder and its
  parents are saved together (a failure part way used to leave the strain
  without its parents).

- After an upgrade, browsers could keep running the previous version's
  scripts for up to an hour. Pages now link static files by a hash of
  their content, so a changed file is always fetched fresh (and an
  unchanged one is cached for good).

- A failed restore now explains itself in your language (it was always
  English), and names the table it stopped on.
- Saving Settings writes every setting together and reloads once; it used
  to reload everything after each of about 15 settings, and a failed write
  could go unreported.
- The sensor graph endpoint refuses a `minutes` range longer than 5,000
  hours instead of misbehaving on huge values.

- Sensor graphs over more than a day showed wrong values. The hourly
  summary they read was rewritten every refresh from the last few minutes
  of its oldest hour, so nearly every hour older than a day held a few
  minutes' average instead of the hour's (on one instance, 95% of hours,
  some off by over 70 units). Fixed, and migration 028 recomputes every
  hour that still has its readings; older history is kept as it is.
- On Postgres, a backup couldn't be restored if any zone had VPD sensors
  chosen (2.5 and later): zones and sensors refer to each other, and the
  restore stopped on the zones table. Zones now go in first without those
  links, which are set once the sensors are in.
- Restoring a large backup on Postgres no longer takes hours. The
  rolling-average trigger ran for every restored sensor reading and got
  slower as the table filled; a 4.6-million-reading restore was on course
  to take more than half a day. The restore now loads readings with the
  trigger off and works the averages out once at the end.
- The same on SQLite, where it was worse: restoring 20,000 readings took
  almost a minute and the time grew with the square of the count. 100,000
  now load in half a second. SQLite's rolling-average trigger also looked
  through a sensor's whole history for every new reading; migration 029
  reads just the latest 16, as Postgres does (same averages).
- Backups now include the hourly sensor summary that graphs over a day
  read from. It keeps history after the raw readings are pruned, so until
  now a restore lost every graph older than the sensor retention period.
  Restoring an older backup rebuilds the summary from its raw readings
  within ten minutes, as before. On SQLite, a restore also no longer
  leaves the replaced instance's summary rows behind.
- Backups made by the Docker image recorded their Isley version as
  "unknown" (they looked for a `VERSION` file that isn't in the image);
  they now record the running version.
- Sensor graphs viewed from a browser in another time zone put their hour
  and day gridlines at the browser's hours and midnights, though labelled
  in the instance's zone (in a half-hour zone such as India, every hour
  tick sat at half past). The chart now places them in the instance's
  zone, through its own date adapter, which replaces the date-fns one.
- The sensor graph's "1 Hour" range had a single time label. Ranges up to
  three hours now have labels every few minutes on round times (:00,
  :10, ...), closer together as you zoom in.

### Security

- An uploaded image is stored with the extension of its actual type, never
  the one in the uploaded file's name, so a picture named `.html` can't be
  served as a web page.

- The CSRF check now covers PATCH requests as well as POST, PUT and DELETE.
- API keys no longer all have full access. The web UI's own endpoints
  (settings, deletes, backups, restore) need an admin key; sensor ingest needs
  the ingest scope. Existing keys are upgraded to admin so current
  integrations keep working; narrow them in Settings → API. Restoring a backup
  made before this change also brings its keys back as admin.

## [2.5.4] - 2026-11-09

### Added

### Changed

- The plants, strains, sensors and activities lists share one set of search,
  filter, result-count and clear-filters helpers (`list-utils.js`) instead of
  four copies. No change in behaviour. (#263, thanks @platentgrows)
- Dependencies: go-i18n 2.7.0 (#260), modernc.org/sqlite 1.61.1 (#261),
  gin-contrib/sessions 1.1.3 (#262).

### Deprecated

### Removed

### Fixed

- Saving a measurement or activity with an earlier date picked on a phone
  failed with "Failed to add measurement". Date pickers leave out the seconds
  (`2026-11-06T18:30`), and the server rejected that format. (#264, #265,
  thanks @dermorz)
- Plant pages no longer get a 500 from `/listLogos` on installs that have never
  uploaded a watermark logo. An empty logos folder no longer breaks the
  decorate-image dialog either.

### Security

## [2.5.3] - 2026-10-25

### Added

- Integration test that round-trips the activity-log xlsx export through excelize,
  so a broken workbook fails CI rather than just a wrong status code.

### Changed

- Dependencies: excelize bumped from v2.11.0 to the v2.11.1 pre-release
  (master @ 2026-10-20), clearing all 15 open Dependabot security alerts. None of
  them were reachable in Isley, which only writes workbooks and never parses
  uploaded ones, but the pinned version carried every one of them. Also
  gin-contrib/sessions 1.1.2 (#256), golang.org/x/crypto 0.58.0 (#257) and
  golang-migrate 4.20.1 (#258).

### Deprecated

### Removed

### Fixed

### Security

## [2.5.2] - 2026-10-25

### Added

### Changed

- Go toolchain pinned to 1.27.3 in go.mod, the GitHub and GitLab test jobs
  and the Docker builder (was 1.25.8 / 1.27.2). Newer dependency releases
  require Go 1.26+. (#253)
- Static assets (`/static`, `/fonts`) now send `Cache-Control`, `ETag` and
  `Last-Modified`, so browsers and CDNs cache them instead of refetching every
  script and stylesheet on each page load. (#251)
- Dependencies: logrus 1.10.2 (#250), testify 1.12.1 (#246), x/text 0.43.0
  (#247), x/image 0.47.0 (#249), modernc.org/sqlite 1.61.0 (#248).
- Dependabot now opens its PRs against `dev` to match the release flow. (#254)

### Deprecated

### Removed

- Two redundant `sensor_data` indexes (`idx_sensor_data_sensor_id`, covered by
  the composite index, and `idx_sensor_data_hourly_bucket`, a duplicate of the
  rollup table's primary key) are dropped by migration 023. (#252)

### Fixed

- Dashboard, plant page and overlay no longer slow down as sensors stop
  reporting. The "latest reading per sensor" lookup scanned past every newer
  row from other sensors; on an instance with ten idle sensors and 4.6M
  readings the dashboard's sensor call took 1.8s and now takes ~2ms. (#251)

### Security

## [2.5.1] - 2026-10-18

### Changed

- Docker builder image bumped to golang 1.27.2. (#242)

### Fixed

- Plant age (Day / Week) now matches between the plants list, dashboard, overlay
  and plant page. Day 1 is the calendar date the plant was started, and the count
  changes at midnight. Postgres and SQLite previously disagreed, and the plant
  page could shift the start date on hosts not running in UTC. (#243, thanks
  @platentgrows)

## [2.5.0] - 2026-10-14

### Added
- New sort options on the plants, strains and sensors pages: Z–A variants,
  Zone, Status and harvest weight (plants), Breeder Z–A (strains), Device,
  Unit and Visibility (sensors). (#240, thanks @platentgrows)
- Sensor ingest accepts a JSON array of up to 60 readings per request. The
  batch is validated as a whole, stored in one transaction, and each reading
  counts toward the 60/min ingest rate limit. (#239, thanks @Rambomst)
- Ingest `429` responses include a `Retry-After` header. (#239)
- The zone leaf temperature offset can be entered in °C or °F (still stored
  in °F). (#239)
- Ingest accepts `celsius` and `fahrenheit` (any case) as temperature units
  and stores them as `°C` and `°F`. (#239)
- Smoke reports: record one or more tasting notes per harvested plant, with an
  overall 1–5 rating, a free-text description, an auto-computed cure duration
  and scores on user-configurable criteria (Settings › Customization). Strain
  pages show the average across harvests, using each plant's most recent report.
  (#228, thanks @CrazyGrower)
- Development data seeder: `go run ./cmd/seed` fills an empty database with
  generated breeders, strains, plants, activity history and sensor readings.
  (#238, thanks @platentgrows)

### Changed
- Confirmation dialogs are standardised app-wide (consistent titles, button
  labels and cancel/escape handling), with a `confirmDelete()` helper for
  destructive actions. Missing translation keys referenced from JavaScript and
  templates were added in all four locales, and a test now keeps the locales in
  sync. The Settings page keeps its active tab and scroll position across
  reloads, and breeders delete in place. (#237, thanks @platentgrows)
- Go module and Docker builder image bumps: x/image 0.45.0, x/text 0.41.0,
  x/crypto 0.55.0, testify 1.12.0, modernc.org/sqlite 1.56.0, golang 1.27.1.

### Deprecated

### Removed

### Fixed
- Column-header sorting and the "Sort by" dropdown now stay in sync on the
  plants, strains, sensors and activities pages via a shared helper. (#240)
- Ingest no longer rejects a reading of exactly `0`. (#239)
- VPD treated a temperature sensor with a lower-case `celsius` unit as
  Fahrenheit. (#239)
- A rejected ingest request no longer consumes rate-limit quota, so an
  oversized batch cannot lock an API key out for the rest of the minute.

### Security

## [2.4.0] - 2026-08-07

### Added

### Changed
- Front-end libraries (Bootstrap, Font Awesome, Video.js, Chart.js with the
  date-fns adapter and zoom plugin, and exif-js) are now bundled and served
  from `/static/vendor/` instead of the jsDelivr CDN. Font Awesome now uses
  the CSS + local webfont build rather than the JS/SVG loader.

### Deprecated

### Removed

### Fixed
- UI styles and icons no longer break when the instance has no internet
  connection; all CSS, JavaScript, and fonts are served locally rather than
  from a CDN (#219).

### Security
- Tightened the Content-Security-Policy to remove the external CDN and Google
  Fonts origins (`cdn.jsdelivr.net`, `cdnjs.cloudflare.com`,
  `fonts.googleapis.com`, `fonts.gstatic.com`) from `script-src`,
  `style-src`, `font-src`, and `connect-src`. The app now loads only
  same-origin assets.

## [2.3.0] - 2026-06-23

### Added
- EcoWitt `ch_ec` support: WH52 soil sensor reporting soil moisture, soil
  temperature, and electrical conductivity (converted to mS/cm), registered
  per channel.

### Changed

### Deprecated

### Removed

### Fixed
- Deleting a sensor that had hourly-rollup data failed with "Failed to delete
  sensor". `DeleteSensorByID` now purges `sensor_data_hourly` and
  `rolling_averages` rows alongside `sensor_data` within a single transaction,
  so the foreign key on `sensor_data_hourly` no longer blocks the delete
  (#206).

### Security

## [2.2.0] - 2026-06-18

### Added
- CannaDB strain import. When enabled under Settings → "CannaDB Strain Import",
  the strain library gains an "Import from CannaDB" button that searches the
  public CannaDB database (cannadb.org) by name and one-click imports a chosen
  strain — name, breeder, indica/sativa lean, autoflower, descriptions, cycle
  time, source URL and parent lineage — into the local library. Records are
  keyed on the CannaDB AT-URI so re-importing updates in place instead of
  duplicating, and imported strains show a "View on CannaDB" link to the full
  record (THC/CBD, yield, terpenes, effects and other fields Isley doesn't
  store locally). The integration is read-only, unauthenticated, and honors
  CannaDB's rate limits with backoff. Disabled by default. An optional API-URL
  override is available for self-hosted CannaDB endpoints.
- Per-zone derived Vapor Pressure Deficit (VPD). When a zone is configured with
  a leaf-temp offset (°F) plus temperature and humidity source sensors, Isley
  computes canopy VPD each poll cycle (Tetens formula) and stores it as a
  regular `derived` sensor reading, so it both displays on the plant page and
  graphs over time. The plant-page VPD chip is colored by the plant's current
  growth stage's ideal range — green (in range), blue + up-arrow (too low),
  red + down-arrow (too high). New per-stage ideal-range editor under
  Settings > Customization (seeded Seedling 0.4–0.8, Veg 0.8–1.2,
  Flower 1.2–1.5 kPa) and new per-zone VPD inputs in the zone edit modal.
  Adds migration 019. (PR #187, contributed by @evelinwa)

### Changed
- CI/build dependency bumps:
  - `actions/checkout` 4 → 6
  - `actions/setup-go` 5 → 6
  - `actions/github-script` 7 → 9
  - `docker/setup-buildx-action` 3 → 4
  - `crazy-max/ghaction-import-gpg` 6 → 7
  - Docker runtime base `alpine` 3.23 → 3.24

### Deprecated

### Removed

### Fixed
- The activity log's empty-state message now centers correctly, matching the
  plants/strains/sensors empty states (the `.activities-empty` block was
  missing the flex centering rules). (PR #185, contributed by @ababoude)
- Completed the French (`fr`) translations for the activity log / notebook /
  export strings (issue #148), which were still placeholder `NEEDS_TRANSLATION`
  values. (PR #184, contributed by @ababoude)

### Security
- Bump golang.org/x/crypto to 0.53.0
- Bump golang.org/x/text to 0.38.0
- Bump golang.org/x/image to 0.42.0
- Bump modernc.org/sqlite to 1.52.0

## [2.1.0] - 2026-06-06

### Added
- EcoWitt sensor support now covers `ch_aisle` (WH31/WN31 multi-channel
  temperature & humidity) and `common_list` outdoor weather-station sensors
  (WH32/WN32, WS80, WH65, WS90 — outdoor T&H, wind, rain, UV, barometric
  pressure, light). These keys appeared in the gateway's `/get_livedata_info`
  response but were previously dropped on scan, so users running outdoor or
  greenhouse stations saw nothing register. Includes defensive handling for
  mixed hex/decimal sensor-ID formats across firmware versions and for
  embedded-unit / dash-placeholder values. (PR #176, contributed by
  @evelinwa)
- The Plants view auto-switches to the Harvested tab on initial load when the
  Living tab is empty — handy after wrapping up a run and inventorying
  harvested plants only. (PR #170, contributed by @anon11215)

### Changed
- The add/edit strain forms now surface the server's validation message in the
  failure toast instead of a generic "update failed" / "Failed to add strain",
  so the offending field is actionable.
- Removed an inconsistent `html.EscapeString` on the `description` field
  returned by `/strains/in-stock` and `/strains/out-of-stock`. The list pages
  don't render the description, but the pre-escape would have seeded a
  double-escape spiral for any consumer that round-tripped the value back
  through a save.
- GitHub Actions release pipeline bumps:
  - `actions/upload-artifact` 4 → 7
  - `sigstore/cosign-installer` 3.5.0 → 4.1.1
  - `softprops/action-gh-release` 2 → 3
  - `docker/setup-qemu-action` 3 → 4
  - `docker/build-push-action` 6 → 7

### Deprecated

### Removed

### Fixed
- #146 (strain regression): v0.2.0's new strain URL validation rejected
  pre-existing URLs stored without an `http(s)://` scheme (common before
  validation existed). Because the edit modal sends the saved URL back
  verbatim, editing **any** field of such a strain — and adding a strain with
  a schemeless URL — failed with a generic `update_error` /
  `Failed to add strain` toast. Schemeless URLs are now normalized
  (`https://` prepended) on add/update and persisted clean; dangerous schemes
  (`javascript:`, `ftp://`, …) are still rejected. The `description` /
  `short_desc` length caps were also relaxed so legacy long free text no
  longer blocks updates.
- `UpdateStrainHandler` now refreshes the in-memory strain cache after a
  successful update — it regressed in the v0.2.0 ConfigStore refactor and the
  UI showed stale strain data until the next restart.
- Follow-up to #146 (strain debugging): the dedicated edit page at
  `/strain/:id/edit` was still showing a generic "Update failed" toast even
  after the previous round of validation work, because `strain-edit.js` was
  missed when `strain-add.js` and the (unused) edit modal were updated to
  parse the server's JSON `error` body. The live editor now surfaces the
  actual validation message the same way.

### Security
- Bump golang Docker base to 1.26.4-alpine3.23
- Bump golang.org/x/crypto to 0.52.0
- Bump golang.org/x/text to 0.37.0
- Bump golang.org/x/image to 0.41.0
- Bump github.com/quic-go/quic-go to 0.59.1
- Bump modernc.org/sqlite to 1.50.1

## [0.2.0] - 2026-04-29

### Added
- Fixed an ordering bug when uploading multiple plant images at once
  where descriptions and dates could be paired with the wrong image. (PR #157)
- The plant detail page now has prev/next arrows in the hero and responds to ArrowLeft / ArrowRight keyboard navigation, cycling within the plant's current top-level state (Living / Harvested / Dead). (PR #158)
- Activities can now be flagged as "watering" and/or "feeding". The built-in "Water" and "Feed" activities are migrated to set the matching flag, and the dashboard "days since last watering / feeding" calculations now use the flags rather than hardcoded names — so renaming or translating the built-in activities no longer breaks tracking. (PR #160)
- Activities can now be linked to metrics. When an activity type has linked metrics, the relevant measurement fields appear on the activity-entry form (single-plant, multi-plant, and edit modals), so values like fertilizer amount or pH can be captured alongside the activity rather than buried in the notes. Required vs. optional metrics are configurable per activity in Settings, and recorded measurements stay linked to the activity that produced them. (PR #161)
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
- All activity write paths (create, edit, delete, multi-plant create) now run inside database transactions. Partial failures no longer leave the activity log in a half-written state, and multi-plant creates pay one WAL fsync instead of N on SQLite.
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
