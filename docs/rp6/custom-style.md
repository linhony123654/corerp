# Constrained natural-language style execution

Status: implemented with local HTTP-fixture end-to-end evidence. This is **natural-language control of the existing fact-preserving presentation capabilities**, not arbitrary literary generation. Live-model interpretation quality remains REQUIRED_IF_AVAILABLE; no configured live model was used. Full RP6 acceptance and the larger goal are not complete.

## Execution contract

An independent style model interprets `prose_instructions` against the resolved/pinned style into a closed seven-field plan: POV, tense, verbosity, dialogue ratio, description density, built-in narrative pack and `unsupported_instructions`. Supported instructions override those controls for this presentation; unspecified settings should remain as supplied. The model cannot return prose, actions, quotes, names, thoughts, budget changes, forbidden-pattern changes or external pack references.

The local renderer then applies the validated plan to the immutable committed facts. Accepted speech is preserved literally, actions remain the same, and original event attribution/order is retained. This closes the factual safety gap that a test for “original quotes still present” in unrestricted model prose would leave open. No keyword mapping implements interpretation in production; an actual compatible HTTP model request supplies the plan. The browser fixture is deliberately a fixed test implementation, not evidence of general language comprehension.

Current capabilities are explicit: person labels; present/past framing; terse/normal/detailed (with known timestamps); normal/separate-line quoted dialogue; known-place annotations when density is positive and verbosity is not terse. Ratios are preferences, not permission to delete speech. Arbitrary literary expansion, sensory details, invented actions, rewritten quotes or inner thoughts are unsupported. The model is instructed to set the unsupported flag whenever part of a request cannot be represented; UI always discloses these limits, and the result additionally warns when the flag is set. A valid plan does not prove an arbitrary requested literary effect was achieved. Full-stage audit must retain that limitation instead of equating constrained control with unrestricted prose.

The planner receives **only the resolved style profile**, including the user's style instructions, not committed facts/dialogue, hidden character context, account data or DecisionContext. Whole presentation-input byte budget is checked first as before. Timeout, network or malformed-plan failure returns a presentation error; it does not silently claim custom success or change canonical settlement. Blank custom instructions use the deterministic renderer with no planner call.

Canonical settlement still saves its deterministic original. A completed primary stream now supplies a page-memory presentation variant after the authoritative observation refresh succeeds (the old path discarded those returned lines). Regeneration uses the same provider. Reload without a pending read returns the saved original; an interrupted settled read retries presentation only. Warnings appear in primary Play as well as regeneration. No model output is stored as new world truth.

## Operator configuration

The server reads these independent variables; it never borrows decision or other tools' credentials:

| Variable | Meaning |
|---|---|
| `CORERP_NARRATIVE_PROVIDER` | `deterministic` (default) or `style_planner` |
| `CORERP_NARRATIVE_ENDPOINT` | Explicit compatible Chat Completions URL; HTTPS required except loopback |
| `CORERP_NARRATIVE_MODEL` | Required explicit model name |
| `CORERP_NARRATIVE_API_KEY` | Required for non-loopback endpoints; keep outside Git |
| `CORERP_NARRATIVE_TIMEOUT` | Total attempt/backoff budget, default10s, maximum30s |
| `CORERP_NARRATIVE_ATTEMPTS` | Default2, allowed1–3 |

Extra narrative settings without explicit planner mode fail startup. Redirects are refused to avoid forwarding credentials/style data. Responses are limited to16KiB. Retry only transient network/HTTP408/429/5xx failures under the total deadline; invalid plans/refusals/tool calls/incomplete output do not retry. Errors omit endpoint, credentials and remote bodies. Remote strict-schema support is not trusted: local parsing rejects missing, duplicate, unknown, null, mistyped, out-of-range and trailing fields.

`observe.narrative_mode` reports the independently configured narrator, separate from `decision_mode`; settings describe the actual capability. Test harnesses explicitly clear inherited narrative credentials/config before using loopback fixtures.

## Evidence

- Core plan tests: actual changed person/tense/layout, literal speech/actions/source IDs, unchanged input/protected fields, invalid plan emits nothing, no planner call on blank instructions, unsupported warning and explicit provider failure.
- Adapter/config tests: real local HTTP429→success, separate Authorization and request schema, style-only payload, strict parser cases, refusal/tool/truncation/size rejection, no sensitive error leakage, redirect prevention, timeout/cancellation and no decision-config inheritance.
- Focused integration: narrative0.157s/core0.012s/storage1.553s/HTTP0.322s PASS. Scoped race+vet: narrative1.180s/core1.022s/storage11.163s/HTTP3.024s PASS. Full narrative/core/HTTP/server:0.142s/0.125s/10.557s/0.147s PASS. Build/typecheck PASS; final small capability-copy follow-up tracked in progress.
- `node scripts/verify-rp1-play.mjs --custom-style --fake-model`: PASS `/tmp/corerp-rp1-e2e-dCa9QH`,5planner calls separate from24decision calls. Actual UI save → first-person/past/separate dialogue while stored original stays second-person; regeneration changes no world/decision facts; injected extra story field rejected; server/page restart resumes presentation only; unsupported instruction warning visible. Actual mobile screenshot inspected. This is not a live-model quality claim.
- Existing all-current-increment deterministic-narrator regression PASS `/tmp/corerp-rp1-e2e-IMEzRX`,33decision HTTP fixture calls; covers the completed-primary-view retention change and all personal views/stream/recovery paths.

Remaining acceptance: audit the bounded custom-style capability against full RP6 requirements, actual100+turn multi-day UI with concurrent world systems and restart, and full-stage verification/checkpoint. Do not mark the full goal complete based on these scoped checks.
