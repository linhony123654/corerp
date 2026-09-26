# F9 — real author packs: design and acceptance ledger

Status: DEFINE/DESIGN; narrative author slice in progress. This stage remains open.

## Scope and authority

Two independently authored, data-only examples must run through the generic Pack platform: a retail career content Pack and a long-form life journal Narrative Pack. Career, Education, wage payment, organization policy, Event, agent decisions, money and law retain their existing owners. A Pack declaration supplies bounded content, never grants a principal permission to perform the declared business action.

Observable acceptance: another valid dataset of each kind installs/activates without Go/Vue source changes; invalid hash, dependency, version or capability fails without partial activation; exact retry is idempotent across restart; failed upgrade leaves the prior active state; disable/unload prevents new use while historical receipts and event references remain verifiable; narrative regeneration changes presentation only. Each claim needs an actual source/reopen/Compare or runtime check, not a manifest-only check.

## Reuse and gaps

- `core.StudioPackageBundle.Validate`, `Store.InstallStudioPackage`, and `ActivateStudioPackages` already provide strict manifest and content hashes, exact dependencies, immutable Event-backed installation, creation-time activation, and a pinned System/Narrative lock. The active narrative profile is consumed by `resolveRPStyle`; settled turns can be read or regenerated with a style override without replaying decisions.
- The current runtime rejects content kind, permits one version per identity, and allows initial activation only. The lock names exactly one System and one Narrative Pack; source replay assumes a single activation. F9 must add a generic content contract and an additive lifecycle source/epoch model before claiming upgrade/disable/unload.
- Retail Pack entries should reference existing organization, position, credential/program, schedule, wage and RP-memory concepts. Materializing an entry must call the existing authorized domain command and retain its source Event. A suggested wage is only a reference value; an authorized Career posting/offer and the existing payroll remain the authority. A failed/disabled Pack cannot erase those Events.
- Career employment currently rejects every world outside the M2 demo scheduler in `AcceptCareerOffer`. The first retail end-to-end route must therefore run in that supported world; using a new Studio world for actual shifts/pay would require a separate scheduler generalization. The Pack registry/activation design must work with this existing world without inventing pay outcomes, while keeping the already shipped Studio creation contract compatible.
- The existing Narrative style supports first-person POV, density `long`, context budget, and bounded prose instructions. The deterministic renderer reads only committed facts and may produce short text for sparse events. A measurable long-form target is not yet a platform field; F9 needs an explicit bounded target contract and a presentation-provider check before claiming that part of the example.

## Increment order

1. Author a standalone life-journal Narrative bundle, load its actual JSON files, validate its hashes and prove creation-time installation/activation, resolved style, read-only alternative rendering and restart. This exercises the existing platform without a host-side package-ID switch.
2. Add the smallest generic content schema and reader. Author a retail dataset that ties job/qualification, schedule, wage reference, training/progression, organization and RP memory to real scoped domain commands and Event evidence. Validate one complete route through the existing owners before expanding entries.
3. Add source-backed package lifecycle and historical pins. Cover failed upgrade, disable/unload, dependency failure, exact retry, restart and Compare/Rebuild. Historical consumers must resolve their original version; active consumers must not use disabled content.
4. Replace both author datasets in the same tests and verify no Go/Vue host edits are required. Run affected race/static checks, stage regression, report and checkpoint before F10.

No remote model, deployment, or production account is part of this stage. A local deterministic/fixture renderer does not prove live-model prose quality.

## Current slice evidence

`docs/f9/life-journal/{manifest.json,narrative.json}` is a standalone authored bundle. `TestF9LifeJournalInstalledActivatedAndRegeneratedWithoutWorldMutation` loads those files, validates the real hash, creates a Studio world, checks exact create retry and active player style, settles a real turn, renders two presentation variants with the same Event IDs and branch head, reopens SQLite, regenerates the variant, and compares projections. Focused storage test PASS. This proves the current Narrative platform slice, not the full F9 lifecycle or long-form quality target.
