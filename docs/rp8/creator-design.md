# Creator workflow: dependency and acceptance map

Status: BUILD, source-backed genesis, conserved participants, spatial preparation, [package installation/activation with consumers](packages.md), [saved player readiness](readiness.md), [recoverable creator API](creator-api.md) and [initial creator/Play UI](creator-ui.md) implemented. Real browser new-world creation/recovery/dialogue/move/wait passes for the bounded two-person/two-place template; full stage/Final acceptance remains open. Original RP8C remains
create world → advanced settings → plugin installation → Narrative/System Pack →
save → Play. Existing Inspector increments do not satisfy this workflow.

## First genesis input contract

`core.StudioWorldSpec` is implemented and consumed by internal
[paused genesis preparation](genesis.md), not yet an exposed creation API. Versioned declaration: name, common existing calendar start,
population2–1,000,000, safe nonnegative opening money/stock,2–32 places,1–64 bounded
bidirectional links forming a connected graph, and2–16 named participants with
exactly one player. Named participants cannot exceed the source population. Names,
local keys, references and duplicates are checked. The first template deliberately
uses the existing2026-09-22 epoch; this does not promise arbitrary calendar migration.

`StudioWorldObjectID` derives stable full-hash identities from spec version, world,
object kind and local key. It prevents template-local names from colliding between
worlds, but is not authorization. Every setting participates in canonical request
hashing. This declaration owns no balances or people: genesis must create the
existing cohort/ledger/inventory sources, then conserve allocations through existing
materialization. Package installation/activation requires separate validated pinned
content; passing world-spec validation does not install a package or grant access.

Creation-authority implementation must use an explicitly provisioned administrative
scope that already exists, with an independent create capability and immutable
provisioning source. The creation request identifies that source scope and a new
target, and validates both request identity and existing authority before target
insertion. Ordinary inspector access and role names never imply create authority.
Do not bypass the existing-branch FK with dangling target grants or add a duplicate
ACL store. Target genesis and its returned recovery receipt must be atomic.

## Verified architectural constraints

- `rp_wait.go:ensureRPWaitIntent` rejects worlds other than the M2 demo; `WaitRP`
  invokes the unscoped `RunAgentLife`. Removing only that guard would schedule the
  wrong world, not enable creation safely.
- `agent.go:RunAgentLife` and `executeNextAgentSchedule` bind clock, queue, branch,
  schedule validation, transit delay, movement commits and audits to M2 constants.
  The authorized wrapper also rejects any other world. Scope must reach every
  read/write/helper, not just the outer method. Existing demo wrappers can remain.
- Dispatch scope also reaches `executeNextM2Economy`, `executeCareerPayroll` and
  `executeCareerLeaveAutoReview`; payroll currently validates/commits using M2
  constants. Generalizing the selector alone is insufficient. Inventory nested
  handlers and their queue producers before allowing a new-world scheduler call.
- First prerequisite implemented: `insertAgentOutbox` derives audience instance
  from the actual immutable source Event and refuses missing Events. Existing M2
  delivery encoding stays identical; a two-existing-world test checks audience/hash,
  reopen, missing-source refusal and unchanged Event heads. This does not itself
  generalize scheduling or prove newly created worlds are playable.
- Internal scoped runner/movement dispatch now threads world/branch through clock,
  queue, movement commits and audits; demo wrappers remain compatible. External
  authorization and Wait guards remain until all nested handlers are ready. Foreign
  non-movement phases are explicitly rejected, not routed into demo-specific
  economy/payroll/leave owners. Tests currently prove distinct existing-world clocks,
  safe foreign-phase denial and no cross-world effects; a real second-world movement
  plus generalized economic/occupational behavior is still required before creation.
- Shared scheduled commit now verifies the complete persisted queue binding before
  writing any effects. Career queue producers derive world/branch from their recorded
  term/contract/obligation owner; they no longer assign all payroll to M2. Execution
  handlers still need corresponding scope propagation; external guards remain.
- Payroll and term activation now obtain scope from the persisted pending queue;
  employment terms follow the contract definition Event. Posting reconciliation,
  term cancellation and effective role grants accept the corresponding scope.
  M2 wrappers remain. Overtime/leave/aggregate behavior still needs its own source
  scoping/calendar work before foreign payroll dispatch can be enabled.
- Accepted overtime and approved leave readers now use contract-origin scope;
  automatic review uses persisted queue scope. Aggregate-exit queue/activation uses
  notice/queue scope, but its M2-specific wage contract and date rules are deliberately
  still enforced. Cross-world execution is not yet exposed. Remaining work includes
  per-world calendar/settings ownership, work-schedule producers, aggregate economy
  definitions and actual second-world action/settlement/replay evidence.
- Movement's transit-delay and superseded-arrival helpers now derive scope from
  persisted queue ownership as well (including closure lookup, retry hash/queue and
  commit). Their former M2 binding was a remaining nested-path gap. Existing delay,
  rollback, later-appointment and replay behavior is regression-tested; this still
  does not replace a successful movement test in a fully source-backed second world.
- `rp_bootstrap.go` hardcodes principal/entity/place/command/Event IDs. M2 bootstrap
  hardcodes currency, SKU, accounts, cohort and grants; these identities occupy
  global primary-key namespaces. Calling bootstrap twice is not two worlds.
- Replay already accepts instance/branch and derives balances, population,
  positions, knowledge and time from source records. New genesis must populate
  existing immutable ledger/inventory/population/movement sources and replay owners,
  rather than clone mutable projections or introduce a second world database.
- `capability_grants` has a required existing-branch FK. Creator role alone cannot
  authorize pre-creation writes. A creation authorization must exist independently
  of the yet-to-be-created target; do not fake a grant on a nonexistent branch.
- The original `PlayWorkspace.vue:connect` fixed M2/Lin binding is now replaced by
  RP7 controlled-entity discovery, authorized selection and per-binding bookmarks.
  Returning from Studio uses a picker hint, never a credential or authority in the
  URL; entering remains subject to separate player authorization. A legacy bookmark
  must be resumed once to learn its binding before it can be safely archived.
- M0 package Manifest schema already specifies identity, kinds, exact dependencies,
  capability declarations, engine API and schema/content hashes. Example manifests
  are documentation, not installed runtime packages. Scoped inventory finds no
  executable backend Extension Registry to replace. Preserve its contract and add
  only the missing runtime installation/activation ownership.
- Narrative style validation currently permits only two builtin refs. An installed
  Narrative Pack must affect validated narrative behavior; saving an arbitrary ref
  or manifest without a consumer is not working pack support.

## Required acceptance evidence

| Requirement | Proof needed |
|---|---|
|Create world|Two independently created worlds coexist; distinct IDs, conserved genesis allocations, valid epoch/clock, no demo-world mutation.|
|Authorization|Explicit creation authority, player/ordinary creator denial, exact target idempotency and mismatch; no role-only escalation.|
|Advanced settings|Bounded validated settings affect existing owners, survive reopen/replay, unsupported fields rejected.|
|Install packages|Manifest/hash/dependency/capability validation; immutable installed content and pinned activation; unknown/code/DB/DOM capabilities rejected.|
|Narrative/System Pack|Actual narrative and system behavior consume the installed declarations, not only catalog labels; independent worlds do not inherit each other's settings.|
|Save|Atomic source-backed creation/activation or explicit recoverable stages, injected rollback, exact retry after lost response; no half-playable world shown as ready.|
|Return to Play|Real creator UI → save → authorized player selection → observe/speak/move/wait in the new world; reload and Runtime restart preserve it.|

## Increment order

1. Generalize existing scheduler scope end-to-end while retaining demo wrappers.
   Prove two-world isolation of due work, clocks, commits and audits before exposing
   new-world creation. This is an RP8C prerequisite, not a new simulation owner.
2. Resolve explicit creation authority and genesis fact contract together. Prefer
   existing capability/Event ownership (for example an explicitly provisioned
   administration scope), but do not commit to a schema until initialization and
   recovery semantics are proven. No implicit grants from ordinary inspector access.
3. Implement idempotent bounded world genesis with namespaced identities and actual
   cohort-derived participants, balanced source records and active epoch/clock.
4. Add declarative package validation/install/activation using existing Manifest
   fields and canonical hashing, with real Narrative and System consumers. No hot
   rule migration requirement and no arbitrary plugin execution.
5. Connect creation/settings/package/save UI and player-authorized Play discovery;
   perform the full real multi-world browser/recovery journey and stage gates.

No permission or external dependency currently blocks this local work. Creation
authority/schema details remain design work, not grounds to narrow the objective.
