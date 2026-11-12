// Package apiv1 is Isley's public JSON API, served under /api/v1. It is an
// adapter over package service: it parses requests, calls a service, and
// shapes the result into the documented contract (openapi.yaml). It never
// touches the database itself.
//
// Conventions (the contract tests hold every response to openapi.yaml):
//
//   - Auth is an API key only, as "Authorization: Bearer <key>" or
//     "X-API-KEY: <key>". Browser sessions are not accepted, so CSRF does
//     not apply here.
//   - Errors are {"error": {"code", "message", "details"}} with a stable
//     code. See writeError.
//   - Lists are {"items": [...], "next_cursor": "..." | null}, with
//     ?limit= (default 50, max 200) and an opaque ?cursor=.
//   - Timestamps are RFC 3339 with the instance zone's offset; calendar
//     days are YYYY-MM-DD.
//   - Field names are snake_case; units are in the field name or a unit
//     field.
package apiv1
