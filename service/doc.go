// Package service owns Isley's data logic: validation, SQL for both
// dialects, transactions, the domain rules (plant age, status meanings) and
// keeping the config-store caches in step with writes.
//
// The web handlers, /api/v1 and the MCP server are adapters over this
// package. No adapter writes SQL.
//
// # Conventions
//
//   - One file per resource (plants.go, zones.go, ...). Each resource is a
//     concrete type reached from Services, e.g. svc.Zones.List(ctx).
//   - Every method takes a context.Context first.
//   - Methods return errors, never a zero value standing in for failure.
//     Failures the caller can act on are *Error values (NotFound, Invalid,
//     Conflict); anything else is an internal error and is wrapped with
//     context.
//   - Dialect differences go through Deps.Dialect, never model.IsPostgres.
//   - Time goes through Deps.Zones and Deps.Clock (see package chrono),
//     never time.Now or time.Local.
//   - Returned types are this package's own, not model/types. Adapters
//     shape them for their audience (templates, the v1 JSON contract).
//   - A write that changes data the config store caches (zones, strains,
//     statuses, activity types, metrics, breeders, streams) refreshes that
//     cache before returning.
//
// Tests run against real SQLite and Postgres databases, as the rest of the
// suite does (tests/testutil).
package service
