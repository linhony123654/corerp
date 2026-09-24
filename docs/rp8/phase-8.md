# RP8 — optional Studio, Creator and Inspector

Status: COMPLETE / local SHIP. Source checkpoint
`86ca6f96d39e4a026f2ff73e48f5823a318cdb34`, clean tree verified, schema030;
original8A/B/C functional
acceptance and final full-backend15968 PASS are reconciled in
[verification.md](verification.md) and [phase-report.md](phase-report.md). RP7 source checkpoint
`c39af6b141b6898eb643799bf380c215c77a1957`, metadata
`7de4b10616a1a7adaa3f1fe973590828737f966b`; clean worktree verified before recon.
Schema028/029 protect [installed package content and activation](packages.md); schema030 protects [saved player readiness](readiness.md). Inspector itself added no migration.
[Read contract](inspector.md), [local Event-backed setup](access.md)
and [actual Studio UI/recovery](studio-ui.md) implemented and increment-verified.
Creator/Play and historical package inspection are implemented and browser verified.
The initial recon/design notes below preserve then-open gaps; current implementation
and resolved dependencies are documented in creator-design.md, creator-api.md,
creator-ui.md and inspector-acceptance.md. Stage verification and checkpoint are
complete; Final Integration remains the next serial phase, not completed by RP8.

## Original acceptance retained

- 8A: optional advanced World Studio; Play remains the default. World/population/
  entity/organization/economy/career/culture/law/plugin/timeline/branch/inspector/
  provider/debug are possible modules, not a requirement to invent empty dashboards.
- 8B: sourced explanations for why/why not, known/not known, rejection, effective
  rule and causal Event. Distinct player, creator and ops/debug permissions,
  including direct HTTP denial rather than only hidden buttons.
- 8C: actual create-world → advanced settings → plugin installation with Narrative
  and Rule/System Packs → save → return to Play. Existing package contracts and
  registry architecture preserved; no arbitrary JS/DOM/database plugin execution.
- Stage gates: focused vertical-slice checks, full normal/vet/relevant race,
  migration/reopen/replay, typecheck/build/configured verifiers and real-browser
  creator/permission/recovery path; report, checkpoint, clean tree before Final.

## Initial recon: verified reuse and then-open gaps

| Surface | Actual source and implication |
|---|---|
|Default entry|`src/main.ts` mounts real Play except explicit `/demo`; keep that boundary.|
|Historical Inspector|`InspectorWorkspace.vue` imports records/entities/packs/whyNot from `src/data/world.ts`; it is a static M0 demonstration, not an authorized live inspector. Preserve `/demo` and its verifiers.|
|World authority|Existing world_instances, branches, rule_epochs, events, commands, audit_records and clocks are reusable. Existing bootstraps use fixed demo identities; do not expose bootstrap as arbitrary world creation or clone rows without lineage/replay semantics.|
|Authorization|Existing principals and branch-scoped capability_grants. `ListVisibleEvents` distinguishes creator world.events.read, operator diagnostics.events.read (payload redacted), player economic visibility. Roles alone are not permission; a creator is not automatically ops.|
|Player evidence|RP7 session-bound context/events preserve own observation/knowledge. Do not reuse the older economic-subject filter for arbitrary RP characters or expose NPC decision inputs to players.|
|Record evidence|Committed Events, rule_epochs, audit_records, observation_records/agent_knowledge and RP decision receipts exist. No generic explanatory HTTP service found; reasons must be selected from recorded evidence, not generated retrospective intent.|
|Packages|M0 packageManifest contract exists in core-contract.schema.json (id/kind/version/engine_api/dependencies/capabilities/hashes/files); example World Packs exist. Scoped backend search found no executable Extension Registry implementation. The frontend extension_registry record is fixture data, not an installed registry. Preserve the contract and confirm implementation inventory before adding only missing runtime pieces.|
|Narrative|Two bounded builtin narrative refs and actual scoped immutable style revisions exist. They are not yet proof of generic pack installation.|

## Initial ordered increments and boundaries

1. Resolve Inspector permission/record contract and deliver one real read-only
   event-evidence slice: authenticate, select authorized instance/branch, inspect
   actual committed Event/rule/causal identifiers, show explicit absent evidence,
   return to Play. Keep raw privileged data outside player state/prompt/storage.
   Test real HTTP/database, no read side effects, revoked/wrong-world/forged access,
   and actual UI refresh/restart. No fabricated “why” when evidence is absent.
2. Extend explanation to recorded rejection/no-op and observer knowledge. “No
   stored knowledge” must not be mislabeled proof of subjective ignorance; report
   observation scope and cutoff. Operator payload redaction remains in force.
3. Resolve world-genesis and immutable declarative package installation together:
   inspect fixed-ID scheduler/economy dependencies, existing canonical hashes,
   Event-backed authority/bootstrap boundaries and package schema. Specify an
   idempotent recoverable transaction before exposing creation writes.
4. Connect actual bounded advanced settings and Narrative/System Pack usage to
   existing owners, persist/reopen/replay and enter the resulting world in Play.
   Reject unsupported capabilities/code; do not silently install inert packs.
5. Full original acceptance audit and stage gates; then Final300turn/30days.

First backend increment DESIGN PASS: explicit role-specific inspector grants with
event/rule fields, one transaction, authenticated strict route and bounded metadata
DTO; no implicit privilege escalation or world mutation. Full stage DESIGN remains
open: arbitrary-world creation requires resolving fixed-world assumptions and initial
creation authority (existing grants require an existing branch). These are
investigation items, not permission to bypass authorization or redefine8C away.

UI direction: separate explicit `/studio`, reuse existing Vue/theme primitives,
evidence-first layout and clearly labeled authority level; no backend detail added
to the default Play HUD. Read visual toolkit before UI implementation. No new
framework, global config, remote publication or production data mutation.

## Recon limitations

Initial recon had no tests; current backend checks are tracked in inspector.md and
the progress log. Searches of guessed
authorization.go / HTTP events.go and docs root globs found absent paths; no code
effect. Continue with actual file inventory/symbol locations, not these guesses.
Semantic search remains unavailable from earlier confirmed environment evidence;
scoped rg fallback used without reinstalling. Overall goal is active and incomplete.
