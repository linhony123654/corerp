# F7 local phase report — information channels, delivery, stance and shared rounds

Status: **local acceptance PASS**. Based on F6 checkpoint
`dc056888b9cdb26abce2f082fa012feb94972c14` in the independent
`f7-information` worktree. The containing scoped commit is the F7 checkpoint;
no push or deployment is included.

## Implemented

- **Unified channel contract and migrations 047–054**:
  - `direct_message`: across locations via phone-like sender/receiver, scheduled delivery, offline/later delivery, recipient-only observation and Knowledge.
  - `rumor`: explicit one-hop interpersonal relay with sender permission (`allow_relay: true`), forward-lineage tracking, second-hop rejection, no automatic truth inflation.
  - `organization`: notice sourced from real Career layoff events (`EndCareerEmployment(kind=layoff)`), redacted public workforce summary, manager-only publication, active-employee-only discovery/access, post-layoff fresh access denial while historical receipt remains.
  - `public_notice`: notice sourced from real enacted-law events (`RPInstitutionFactRecorded(kind=law_enactment)`), legislator-only publication, public list/access, no debug truth injection.
- **Recipient stance tracking**:
  - Controlled recipients can record `believe`, `doubt`, or `reject` on delivered claims.
  - Append-only corrections preserve belief history; underlying world truth is never rewritten.
- **Decision & model boundary**:
  - Delivered messages populate `life.information` in `RPLifeContext` with observer-scoped handles, stated reliability, and channel context; canonical Entity IDs and raw Event IDs are excluded.
- **F3 shared-round information ingress (7 actions)**:
  - `information_send`, `information_stance`, `information_relay`, `information_public_access`, `information_organization_access`, `information_public_publish`, `information_organization_publish`.
  - Exact selected-child command reservation, Human-gated selection, Event-before-receipt crash recovery, and direct-bypass protection.
- **Purpose-scoped publication source discovery**:
  - `rp_information_publication_sources.go` exposes session-bound opaque handles for authorized laws and manager layoffs; strict HTTP/MCP rejects raw source Event IDs.
  - MCP tool registry expanded to 30 tools with strict handle-only schemas.

## Verification evidence

- `go test ./internal/storage ./internal/transport/httpapi -run '^(TestRPInformation|TestRPSharedInformation)' -count=1`: PASS (storage 15.599s, HTTP 7.279s).
- Scoped race detection `go test -race ./internal/storage ./internal/transport/httpapi -run '^(TestRPInformation|TestRPSharedInformation)' -count=1`: PASS (storage 256.844s, HTTP 85.092s).
- Fast packages test suite `go test -buildvcs=false ./cmd/... ./internal/core ./internal/decision ./internal/narrative ./internal/transport/httpapi`: PASS (all passed, longest HTTP 44.905s).
- Real MCP stdio integration test `npm test` in `clients/mcp`: PASS (4/4 passed in 7.878s).
- Scoped `go vet -buildvcs=false ./...`, `git diff --check`, and changed-file `gofmt -l`: clean PASS with zero errors.
- Full storage test suite `go test -buildvcs=false ./internal/storage -count=1 -timeout=40m`: PASS (storage 1575.876s).

## Limitations and deferred work

- Public notice discovery lists at most 20 recent rows without a continuation cursor; this is a bounded display window.
- One-hop rumor policy requires explicit sender permission (`allow_relay: true`); second-hop forwarding is rejected by design to prevent unconstrained gossip amplification without truth grounding.
- MCP information tools are verified for registration, schema validation, routing, and missing-round denial; no live external model information action is claimed.
- No production or external-provider integration was requested or exercised.
