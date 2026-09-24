# Declarative Studio package installation

Status: installation, initial activation and narrative/NPC-budget consumers implemented.
Installing a package does not activate its rules, grant capabilities or make a
world playable. No HTTP/UI installation endpoint or external package fetch is
introduced by this increment.

## Preserved contract and supported subset

The Manifest preserves all fields of
[`packageManifest`](../m0/core-contract.schema.json): schema_version, id, kind,
version, engine_api, requires, optional, capabilities, schema_hash, content_hash
and content_files. The runtime accepts the existing M0 manifest/engine version
`m0-draft-2026-09-22` and its verified canonical schema hash.

The exchange schema's world/content/adapter kinds remain valid contract concepts,
but this first runtime installer rejects them explicitly. Supported bundles:

|Kind|Single content filename|Allowed capability|Typed payload|
|---|---|---|---|
|system|system.json|rules.npc.daily_budget|NPC daily action budget, 1–64|
|narrative|narrative.json|narrative.style|Validated existing RPStyleProfile|

The content object has version `corerp.studio-package.v1` and exactly its typed
`system_rules` or `narrative_style` section. Content hash is CoreRP canonical JSON
SHA-256 of this single document, excluding the Manifest, matching M0's single-file
hash convention. No extraction paths, JS/WASM, SQL, URLs or executable callbacks
are accepted. Narrative selects an existing supported presentation renderer;
arbitrary NarrativePackRef strings are not accepted as a substitute for a renderer.

Identity/version syntax follows the existing contract with bounded exact versions.
Dependencies are explicit arrays, at most32 entries combined; duplicate/self
dependencies, missing required dependencies, exact-version mismatches and cycles
are rejected. Present optional dependencies participate in validation, including
when a later install would invalidate an earlier optional declaration. Missing
optional dependencies are allowed. At most32 packages/world and64KiB canonical
bytes/bundle; one version per package identity, no hot replacement.

## Ownership, authorization and recovery

`Store.InstallStudioPackage` accepts the exact saved genesis declaration, target
binding and bundle. It rechecks sourced world.create before retries, rejects a
foreign target/principal, and requires spatial preparation, the original paused
clock and a pending-activation epoch for a first install.

`StudioPackageInstalled` saves the full validated bundle and genesis reference
in the existing private Event transaction. There is no second package database
or filesystem cache to diverge. Exact retry returns the original receipt; changed
requests, duplicate identities and dependency conflicts cannot overwrite it.
Installation emits no public Outbox and does not modify another world's registry.
The later activation must pin these exact saved contents and validate the entire
selected dependency set before using them.

Migration028 adds narrowly scoped UPDATE/DELETE/REPLACE guards for package Event
content. Legacy non-package Event behavior is unchanged. This was necessary:
the initial adversarial test proved the previous Event table did not itself
prevent direct SQL mutation. The INSERT guard also covers SQLite REPLACE's
implicit-delete behavior and all three Event uniqueness keys. This is not a claim
that a filesystem administrator capable of dropping triggers cannot edit a DB.

The migration adds no tables or domain state. Upgrade is automatic at Open using
the existing migration owner; the SQL contract is
[schema-028-package-content.sql](schema-028-package-content.sql). Upgrade/reopen
tests preserve legacy Event counts and verify each guard exists exactly once.

## Verification and remaining work

Core tests cover contract identity, typed content, hashes, capabilities, paths,
dependency versions and cycles. Actual SQLite installation tests cover rollback,
two-world isolation, saved content, immutable writes, exact retry after
rebuild/reopen and revoked-authority denial. M0 verifier still passes52 checks.
Final selected race passed: core1.064s/storage35.634s, including existing migration
and request-retirement upgrade tests; wholebackend `go vet ./...` passed.

Player binding/readiness is now [implemented separately](readiness.md). Next: creator UI/API and the complete save→Play recovery
journey. Package behavior below is verified through its runtime readers, not a
completed browser creation journey.

## Initial activation and actual consumers

`ActivateStudioPackages` selects exactly one installed System and Narrative Pack,
validates their selected dependency set and pins each identity/version, content
hash, manifest hash and installation Event. The lock also names the scoped-agent
algorithm version, movement phase and deterministic conflict order. Hashing the
entire lock yields the new ruleset hash; package capabilities remain declarations,
not permissions conferred on a principal.

The activation Event belongs to epoch_0. One transaction closes epoch_0 at the
following sequence and inserts the new half-open Rule Epoch starting there.
Only the original paused pending-activation world can activate; no hot upgrade.
Current creation authority is rechecked before retries. Rollback cannot leave
closed genesis without its successor. World lifecycle stays paused and no player
control/credential is issued: package activation is distinct from Play readiness.

The existing narrative resolver now uses the pinned Narrative profile as the base
for a Studio world; existing world/session/scene/per-call style overlays keep
their precedence. Turn-style pins retain the installation Event as provenance.
Legacy non-Studio worlds retain the session POV/default style behavior.
This uses the existing presentation-only RPStyleProfile and supported renderers;
no hidden NPC input or ability to rewrite facts is added.

The existing `rpInitiativeEligible` gate reads the pinned System daily action
budget while retaining the hourly cooldown and world/actor-scoped daily count.
It does not mutate profiles or create a second character state. Other worlds
continue using their own profiles/packages. Tests exercise the actual predicate
with one recorded daily-action fixture: budget1 rejects further action while
budget2 permits it, despite both underlying profiles having the same budget8.
This test does not claim the production Wait/player-binding workflow is complete.

Consumers verify the current epoch hash/range, immutable activation Event and
exact installed pins before use. Missing or inconsistent Studio content cannot
silently fall back to demo defaults. The existing projection comparison/rebuild
owner detects and repairs the Studio epoch range/lock from the activation source.
No online model or package re-download is needed for that repair.

Additive schema029 protects activation Events against UPDATE/DELETE/REPLACE,
without changing applied028. See [migration contract](schema-029-package-activation.sql).
Normal activation tests cover two-world consumer differences, half-open sequence
boundary, rollback, epoch corruption detection/repair, rebuild/reopen, exact retry
and revoked authority. Separate [readiness/player grants and scoped Wait](readiness.md) are now implemented; broader cross-world career dependencies,
link-minute consumption, full UI and stage-wide acceptance remain pending.

Final current-source Studio/style/initiative/legacy-upgrade race passed60.743s;
wholebackend `go vet ./...` passed. These are increment checks, not the full
RP8 or Final Integration gate.
