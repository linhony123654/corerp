# Repository Guidelines

## Project Structure & Module Organization

CoreRP is a Go monorepo for a persistent narrative runtime. Main entrypoints live in `cmd/` (`cmd/corerp` for the app, `cmd/debugrank` for utilities). Runtime logic is under `internal/`: `runtime/` orchestrates turns, `events/` owns event sourcing and replay, `state/` projects world state, `actions/` executes `ActionFrame`s, `agents/` handles identity/director logic, `memory/` and `emotion/` manage recall and pressure systems, and `api/` exposes HTTP/SSE routes. Web assets are in `web/`. Authorable world and character content lives in `worlds/` and `characters/`. Architecture and session records are tracked in `ARCHITECTURE*.md`, `README.md`, and `SESSION_LOG.md`.

## Build, Test, and Development Commands

- `go build -o corerp ./cmd/corerp` builds the main binary.
- `./corerp serve -characters ./characters -secure-cookie=false` runs the local server.
- `go test ./...` runs the full Go test suite.
- `go test -race ./...` checks for race conditions; use before merging runtime changes.
- `node --check web/app.js` validates frontend JavaScript syntax.

## Coding Style & Naming Conventions

Use standard Go formatting (`gofmt -w`) and idiomatic Go naming: exported identifiers in `CamelCase`, internal helpers in `camelCase`. Keep packages cohesive; runtime orchestration belongs in `internal/runtime`, not `api` or `web`. Prefer ASCII in source files unless the file already contains localized content. Frontend code is plain JavaScript; keep logic modular and avoid moving runtime rules into `web/`.

## Testing Guidelines

Tests use Go’s built-in `testing` package and are colocated as `*_test.go`. Name tests by behavior, e.g. `TestDirectorAutoChainBuildsRoleBasedSteps`. For runtime work, cover both deterministic behavior and cross-step effects. If you touch concurrency, replay, or multi-step execution, run `go test -race ./...` in addition to targeted package tests.

## Commit & Pull Request Guidelines

Recent history follows conventional prefixes: `feat:`, `fix:`, `refactor:`, `docs:`. Keep commit subjects short and imperative, for example: `fix: downgrade out-of-role step actions`. PRs should describe user-visible behavior, runtime implications, and tests run. Include screenshots for `web/` changes and note any required doc updates when APIs, architecture, or workflows change.

## Architecture & Documentation Notes

Preserve the core invariant: world state changes only through committed events. When changing architecture, APIs, or workflows, update `README.md`, relevant `ARCHITECTURE*.md` files, and append an entry to `SESSION_LOG.md`.
For future-state design, treat `FINAL_ARCHITECTURE_BLUEPRINT.md` as the canonical target document rather than expanding `README.md`.
