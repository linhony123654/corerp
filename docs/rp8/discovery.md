# Authorized Studio scope and event discovery

Implemented in the existing Store/HTTP/Studio layers, not another world catalog.
All endpoints are authenticated strict JSON POST, bind the principal to bearer
identity, use no-store headers, and read from one SQLite snapshot without mutation.
The same grant predicate now protects event detail and timeline/scope readers.

## Routes

- `/api/v1/studio/scopes/list`: optional `after:{instance_id,branch_id}` and
  `limit` (default20, max50). Only active creator/operator principals; other roles
  denied. Result `scopes` includes only branches with the matching role-specific
  active inspector grant, branch-or-wildcard subject and both event/rule fields.
  Returns actual label/head/access level and optional `next_after`. `EXISTS` avoids
  duplicate branches when several qualifying grants overlap. Active ungranted
  creators receive an empty array, not a global world list.
- `/api/v1/studio/events/list`: exact `instance_id`, `branch_id`, optional `limit`
  (20/50), `through_sequence` and `before_sequence`. Zero anchor means current head;
  zero before means the first descending page. Positive values are bounded safe
  integers; future-head/after-anchor cursors are rejected. Result pins
  `through_sequence`, includes at mostlimit Event ID/sequence/type/world-time
  summaries, and optional `next_before_sequence` from the last emitted record.
  No payload, actor, command, raw audit or personal knowledge is listed for any role.

Pagination positions are ordering hints, not permission tokens. Every page verifies
current permissions. Scope pages reflect current grants (not a persistent grant
snapshot); Event pages can be pinned to immutable historical head while a newer
Event is appended. Restart retains that historical page set. Event detail rechecks
permission independently when the user selects a row.

## Actual UI

Input only the inspection token, choose “列出授权世界”, select a returned branch,
page the timeline and select an Event. No world/branch/Event ID transcription is
required for the common path. Manual fields remain for exact known references.
Timeline pages contain5rows to keep the inspector readable; the timeline collapses
when detail is displayed and can be reopened. Explicit refresh reads a new head.
Empty/no-grant/error/loading states are explicit. Credential changes clear all
lists/results; branch changes clear timeline/detail. Requests have cancellation,
revision isolation and memory-only storage. Auth failures clear stale lists/results.
No permission-setting control or new default-Play HUD is introduced.

## Evidence and remaining scope

Focused71122 PASS storage0.542s/HTTP0.156s: multiple granted branches/keyset pagination,
empty ungranted scope, revoked permission between pages, pinned descending Event
pages while newer Event commits, no duplicate/omitted history, bounded metadata,
ops revoke, cross-world/future cursor/player rejection, reopen and unchanged head.
HTTP tests exercise actual scopes→timeline→detail with identity/role denial.

Build86103 PASS1.80s. Expanded browser15733 PASS, then final current-source75810
PASS build1.88s, actual browser `/tmp/corerp-rp8-studio-67FE2q` and M0 52checks.
The final browser run includes delayed real directory response after credential
clear, original detail/restart/revoke/role checks, five→ten Event pagination and
selection without typed IDs. Final desktop/mobile screenshots inspected; prior
non-overlap and horizontal overflow assertions pass. All owned processes terminal.
Backend25270 PASS: related storage race6.073s/HTTP2.569s/admin3.055s, whole backend
vet and full HTTP/server normal12.211s/0.155s. Gofmt/whitespace PASS.
No full RP8 gate pass; complete rejection/
no-op/knowledge explanation and original8C creation/packs remain mandatory.
