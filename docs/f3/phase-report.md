# F3 phase report — external residents and shared world time

Status: **F3 acceptance PASS — scoped checkpoint in the containing commit**. F2 source checkpoint:
`0daf9f238f66d99b63bf3aa8f73a7c2ac7d2db16` (`main`). This report records
local F3 evidence and two four-decision live gateway runs, not a release or a
claim of independently audited upstream provider usage.

## Delivered boundary

- Migrations 036–041 add sourced, replayable service-controller enrollment,
  exclusive assignment, monotone release and explicit cross-principal
  replacement, plus application-only Human-gated shared rounds. Assignment
  binds a distinct service principal, controller instance, Entity, session and
  generation. An old controller cannot submit a fresh proposal, even if its
  principal is later reassigned. Disconnect does not impersonate the resident
  or cancel previously accepted scheduler obligations.
- The existing Event, typed Move/Speech/Wait/Turn and scheduler owners remain
  authoritative. A round accepts private speech, immediate move or absolute
  wait submissions at one observed baseline. Human and all assigned service
  residents must respond before it selects one action or a single Human-owned
  nearest-boundary wait. Unselected actions are explicitly deferred. Selection
  rotates across Entities; a repeated external action cannot starve a Human
  wait at unchanged world time. Selected speech drains the existing NPC turn;
  selected movement commits one existing movement Event. Persisted selection,
  original child key and payload allow exact recovery after a lost response.
  A wait selected but not yet accepted is marked stale if the baseline changes;
  accepted intents may continue through scheduler Events. Round opening rejects
  pre-retired or preaccepted derived child keys, and active rounds reserve those
  keys before proposals. A shared advancement budget only limits the present
  scheduler call; retrying with a different budget preserves the exact accepted
  wait identity, including compatibility with older budget-bound wait intents.
- Exact accepted-key results remain available to their original principal and
  session after a quiescent handoff, without reviving fresh old-generation
  authority. Completed round receipts freeze their source Event time; raw
  another-resident proposals and canonical identity are not disclosed.
  External `service` principals cannot read raw AgentKnowledge even if given a
  legacy referral capability. That internal, explicitly authorized referral
  contract remains unchanged.
- Bearer-bound HTTP routes and strict MCP participant tools expose the same
  scoped sessions, observations and shared-round operations. A trusted local
  operator CLI provides sourced controller enrollment/assignment/release/
  replacement; it neither issues bearer tokens nor gives models operator
  authority.

## Verification ledger

| Gate | Evidence | Status |
| --- | --- | --- |
| Authority and privacy | Focused enrollment/assignment/release/replacement, adversarial raw-knowledge, old-generation/foreign-principal and accepted-key tests; Compare→Rebuild→Compare and process reopen | PASS in focused storage runs |
| Human barrier and shared time | Three-resident wait/speech/move fixtures: withheld Human, one clock Event, scheduler arrival, conflicting private actions, two-round rotation, lost-response recovery and direct-command bypass rejection | PASS in focused storage/HTTP runs |
| Actual client transport | `clients/mcp/npm test` on corrected source (2/2, 19.272s): SDK stdio→HTTP→SQLite, two service credentials plus Human; scripted consecutive speeches, hearing/leave, release/frozen receipts and different-budget replay | PASS **fixture**, not live Provider |
| Opt-in external-resident driver | Current-source `clients/mcp/npm test` PASS4/4 (6.529s): local fake chat-completions endpoint through four-call driver, real separate MCP stdio sessions and Human-gated rounds. Each accepted receipt is matched to the authoritative Event sequence, actor, type and exact model text; each next model input includes only bounded speech text heard on its own filtered event stream, not raw IDs or private Context. Provider response metadata is never sent through a resident MCP tool. JSON Mode and bounded completion-budget options are covered. | PASS **fixture** |
| Full RP storage regression | Final-source `go test ./internal/storage -run '^TestRP' -count=1 -timeout=20m` PASS260.817s after the wait recovery tests and live-adapter work. Earlier corrected-source PASS288.474s; older 302.310s run predates recovery fixes. | PASS |
| HTTP and controller CLI | Full `go test ./internal/transport/httpapi ./cmd/corerp-controller -count=1 -timeout=10m` on corrected source (23.804s/0.463s); focused RP HTTP PASS6.780s | PASS |
| Relevant race | Corrected-source `go test -race ./internal/storage ./internal/transport/httpapi -run '^(TestRPShared|TestRPController|TestRPExternalController|TestRPRequest)' -count=1 -timeout=10m` (214.658s/14.333s) | PASS |
| Wait crash/key/budget recovery | Four new `TestRPSharedRound*` cases cover stale-after-reopen, pre-retired/preaccepted child keys, and changed-budget current/legacy wait intent; PASS1.668s. Affected controller/round/request storage PASS21.012s; final broad RP regression PASS260.817s | PASS |
| Static and build | Corrected-source `go vet ./...`, all-package no-test compilation, `git diff --check` and empty `gofmt -l`; frontend `npm run build` (vue-tsc + Vite) with no frontend changes | PASS |
| Live two-resident decisions | User-authorized HTTPS gateway `cpa.linhony.xyz`: first `gpt-6-astra`, then user-confirmed Step model `step-3.7-flash`, each in a separate disposable world. Both runs completed A/B/A/B, two responses and two accepted Speech Events per role, with exact response-text/Event checks and B hearing A. Read-only SQLite audits confirmed Events 15/19/22/25, actors A/B/A/B, and increasing world time in both worlds. | PASS live gateway execution |
| Provider-route provenance | User-exported relay-backend request log contains four successful Step-routed `POST /v1/chat/completions` calls in the successful world's wall-clock interval, with one configured source/key and sequential timings matching the driver's A/B/A/B loop. See scoped log review below. | PASS relay-route corroboration; upstream billing and response-ID equality NOT VERIFIED |

The live run used the bearer credential already configured for the active
`cliproxyapi` Codex provider, passed only in the child process environment.
No credential was printed or stored in this repository. A first attempt against
`api.openai.com` using the existing `OPENAI_API_KEY` failed at TCP transport
before any HTTP response; it is not counted as a model decision. The configured
gateway returned these bounded completion receipts (its `x-request-id` header
was absent):

| Resident decision | Gateway response ID | Speech Event | World time |
| --- | --- | ---: | --- |
| A1 | `resp_0f147bdcfc861bd7016ab65933ca6487d2b16ab7692c85782e` | 15 | 02:03Z |
| B1 | `resp_0f9817e5b4bd21c1016ab659373b1c87d28df4848a1876e354` | 19 | 02:04Z |
| A2 | `resp_041a4992b5f717b8016ab6593c6ce487d2841ba1d3a2763aa4` | 22 | 02:05Z |
| B2 | `resp_0600b1485a068c31016ab6593f28e087d1a8ef4fcf199d192c` | 25 | 02:06Z |

The disposable audit world is `/tmp/corerp-f3-live-hFOSIj/world.db`; this is
local temporary data, not a durable backup. Response IDs and reported model
names are gateway-returned metadata. They do not independently prove which
upstream model executed or reconcile provider billing.

The user confirmed that the free Step allocation is reached through the same
configured relay key. Two `step-5-preview` attempts at the original 300-token
cap and one `step-3.7-flash` attempt at 1024 tokens returned
`finish_reason=length` and were rejected before any model speech Event. The
successful Step run used `step-3.7-flash`, `response_format=json_object` and a
4096-token cap; the adapter still checked exact JSON fields, bounded text,
finish reason and each authoritative Event. Its separate disposable audit
world is `/tmp/corerp-f3-live-kYFVbz/world.db`:

| Step resident decision | Relay response ID | Speech Event | World time |
| --- | --- | ---: | --- |
| A1 | `d84ddbc06d54bd02e786f500163206f6.53d304af0e41528048b05d5b9be7df7e` | 15 | 02:03Z |
| B1 | `4bd49dedecd43c75643dfa03cf2191c7.72ad7c141940a9f01e704ac91be564bd` | 19 | 02:04Z |
| A2 | `bd70767785b0241446329089dd406bc7.52c3af37cc269798311939b838d81004` | 22 | 02:05Z |
| B2 | `ff14d9dd2e69d38711e0ac79f62ba14e.4e1c8892b168a65108ad22b9ef4974cd` | 25 | 02:06Z |

Each of these gateway responses reported `model=step-3.7-flash`; none supplied
an `x-request-id` header. The project did not receive a separate direct
`api.stepfun.com` credential. This is a live Step-labelled relay run, not a
claim of independently verified upstream Step usage or free-credit accounting.

The user's relay-backend JSON export contains 85 request Events; exactly four
successful `step-3.7-flash` `POST /v1/chat/completions` calls fall within this
successful disposable world's wall-clock window. All four have the same
configured source/key, `OpenAICompatExecutor` and `node` client. The local
driver awaits each model response and round settlement before requesting the
next A/B/A/B decision. The relay rows therefore align as follows (times in
Asia/Shanghai; resident/Event mapping is inferred from serial order, **not**
matched by response ID):

| Relay log ID | Request start | Latency | Inferred decision | Accepted Speech Event |
| --- | --- | ---: | --- | ---: |
| 24427 | 19:34:52.012 | 12.658s | A1 | 15 |
| 24430 | 19:35:05.157 | 8.983s | B1 | 19 |
| 24431 | 19:35:14.565 | 4.718s | A2 | 22 |
| 24434 | 19:35:19.697 | 13.803s | B2 | 25 |

The successful SQLite world was created at 19:34:50 and last modified at
19:35:33.747, just after the fourth relay request's estimated end at
19:35:33.500. A preceding `step-3.7-flash` HTTP-success row at 19:34:20 belongs
to the rejected `finish_reason=length` attempt and produced no accepted speech.
This exported request log independently corroborates all four gateway-routed
model calls, but has no provider response ID: exact row-to-response equality
and upstream Step billing/accounting remain unverified. The export itself is
not committed, and no unmasked credential, account address or client IP is
retained in this report.

## Gate and scoped limits

The goal's F3 live-MCP/Provider path now has two separate four-response HTTPS
gateway runs, each correlated with four authoritative Events. Scripted MCP
fixtures and the deterministic internal NPC provider are not counted as those
decisions. The configured gateway endpoint/model and all four independent
relay request rows have been reviewed under the provenance contract in
[`clients/mcp/README.md`](../../clients/mcp/README.md). This satisfies the
F3 live external-resident acceptance at the configured gateway level, without
claiming direct upstream Step response-ID or billing reconciliation. The
current-source broad regression passed. The containing scoped commit is the F3
checkpoint; F4 begins only after its clean recoverability audit. The F1 live
long-form quality check is also pending, independently of F3.

Round proposals currently include speech and **immediate** movement, not social
commands or timed journey starts. Those typed actions remain available through
their existing owners outside an active shared round; they cannot bypass an
open round. No automatic lease/failover, public MMO, service-token provisioning,
or per-model polling loop is claimed. Explicit operator release/rotation is the
implemented policy. The independent recovery-contract review and corrected-
source focused/race checks are finished. Direct upstream Step accounting and
exact relay-log/response-ID reconciliation remain unverified; neither is
claimed by this F3 gateway-level acceptance.
