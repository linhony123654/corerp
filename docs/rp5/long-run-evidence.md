# RP-5 same-world long-run evidence

`TestRPOpportunitySameWorldFortnightDivergenceAndReplay` executes four copies of one actual SQLite world: three fixed production opportunity streams and one exact repeat of the first. This is not the earlier RP-2 random-provider fixture. Every provider invocation delegates to the production deterministic provider; chance, eligibility, receipts, pressure, cadence and committed effects use the real RP-5 service/storage path.

## Scenario and causal comparison

The initial world uses the existing life economy, four named people plus a fifth person Nora conserved from the cohort. Remaining aggregate population is15. Nora has the existing immutable sociable identity, actual cash/inventory, a declared residence, a bounded first-week home/cafe routine and genuine initial gifts/encounters with Lin. Lin then remains elsewhere. Bo's test-only control grant supplies player authority, not fabricated business facts. Every run uses the same daily movement/Wait rules and gives Nora one real unit of money only when Bo actually meets her at lunch. No gift is submitted for an absent person.

In the initial normal invocation, replay hash is identical in all four snapshot copies:
`sha256:f1db698f50ea50b34843fc248ae5c22e8c6659ab479734548ce5d989fbe59c6a`.

| Recorded stream | Actual optional visit | Subsequent lunch gifts | Final Nora cash | Nora→Bo trust | Quiet Waits | Rare evaluations/hits |
|---|---|---:|---:|---:|---:|---:|
| life-stream-a | Sep27 18:15 UTC | 0 | 268 | 0 | 111/112 | 18/0 |
| life-stream-b | Oct02 10:15 UTC | 5 | 273 | 5 | 111/112 | 18/0 |
| life-stream-c | Oct04 07:15 UTC | 3 | 271 | 3 | 111/112 | 18/0 |
| life-stream-a repeated | Sep27 18:15 UTC | 0 | 268 | 0 | 111/112 | 18/0 |

The first visit occurs after lunch and is followed by the same authored routine. Later streams make Nora available for different subsequent real lunch encounters after her initial schedule has ended. Thus immutable movement/encounter history, money and relationship values diverge, not merely dialogue text. The cash differences equal actual gifts, and trust equals accepted gift count. Same-stream repetition compares the full outcome, including final replay hash and provider-call count, and is exact. Stream a's final hash is `sha256:69cd44a6782dfaed0c35340369109fd5d32cfde2977172bb721943fb5d0b4c79`.

## Consistency and ordinary life

Each run spans14 world days and112 completed service Waits, with exact retries after every Wait, a database close/open/rebuild on day7 and clean projection comparisons every day. All20 people remain conserved. The existing economy produces14 wage obligations and14 actual consumption outcomes. Provider calls stay within the per-Wait HOT bound and WARM stays within its rule-decision bound. Retries do not repeat provider work. Actual optional departures must have selected own-source visit receipts, not arbitrary test-provider movement.

“Quiet” here means no accepted initiative speech/departure in that Wait; it does **not** mean wages, scheduled movement, consumption or other ordinary simulation stop. Each run has111 quiet Waits and one actual optional outing. Rare opportunities stay at≤100bp without quiet/seeking bonuses. The observed18 rare evaluations per run all miss; this verifies an absence of a rare-event floor, not a statistical estimate of rarity. Separate rare hit/miss integration proves that eligible rare visits can actually occur and never promise the friend's presence.

No source in this scenario authorizes a new Career transition, and the test rejects invented `RPCareerFactRecorded` Events. The scenario is not a population estimate of major-life-event incidence. Own actual layoff/termination pressure and other supported consequential changes are verified by their existing source/pressure tests. No health model or random illness is fabricated.

## Verification and limitations

Initial complete four-run normal check PASS111.147s. Explicit receipt→departure causality and no-invented-Career assertions were added afterward; current full normal suite PASS (storage352.593s) and full vet PASS include them. Final isolated full four-stream race test PASS5159.82s (package5160.833s), including parent equality/divergence assertions; other225 storage tests separately PASS4058.708s. All226 top-level tests reconciled with no skips/failures/duplicates. See race-verification.json and phase-5-audit.md for the original full-run timeout and exhaustive replacement execution.

The final race invocation reproduces the table's actual visits, gifts, money, trust, quiet counts and rare outcomes. Its four copies share initial hash `sha256:a432f9d1870010984e72e1466c443e21a3d78745433f89c20702835c9ed6a712`; a and a-repeated share final hash `sha256:c00e066d2d1b56c891b8c803d6b7a7e6af84516f0d2d3449959825edfd1d38af`. These are invocation-specific fixture hashes; the claim is exact repetition from the same actual snapshot, not identical hashes for independently initialized test invocations.

This is a deterministic local provider integration test, not a live LLM evaluation, human play study, frontend100-turn RP-6 acceptance or final300-turn/30-day multi-client integration. Richer off-scene visits, perpetual routine authoring and additional event types remain outside this minimal source/LOD slice; existing authority is not weakened to force those effects.
