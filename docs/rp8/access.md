# Local Studio inspector provisioning

Implemented: `backend/cmd/corerp-admin`, local-only explicit grant/revoke with
Event-backed recovery. Not a public HTTP endpoint, MCP tool or browser permission
toggle. The local filesystem administrator must already have access to the target
database. No production database has been operated on during implementation.

## Operation

From `backend`, run `go run ./cmd/corerp-admin` with all required flags:

| Flag | Meaning |
|---|---|
|`-db`|Existing regular SQLite file; absent path is rejected, never bootstrapped.|
|`-operator`|Existing active operator principal for attribution. Its bearer token alone cannot call this local operation.|
|`-instance`, `-branch`|Exact existing world and branch.|
|`-target`|Existing active creator or operator; players/services/agents rejected.|
|`-status`|`active` to grant, `revoked` to revoke.|
|`-expected-head`|Actual current branch Event sequence, checked atomically.|
|`-key`|Unique durable request key; preserve every flag when retrying a lost response.|
|`-explain`|Optional, false by default. Explicitly adds bounded diagnostic/observation explanation reads.|
|`-purpose create_world`|Optional independent `world.create` grant/revoke, creator-only, field `genesis`; cannot combine with `-explain`. The source administrative branch must already exist.|

Creator targets receive only `world.inspector.read`; operator targets receive only
`diagnostics.inspector.read`. Both are branch-subject scoped with exactly event/rule
fields by default. `-explain` adds the `explain` field on the same grant; existing
grants do not acquire it automatically. Changing this choice requires a new request
key and current expected head. Creator explanation access includes recorded observer
metadata, not private claim payloads; operator access redacts observer and NPC details.
This does not grant world creation, administrative writes, a player identity,
knowledge mutation or provider configuration. It creates neither principals nor tokens.

The default inspector operation does not grant creation. Explicit
`-purpose create_world` instead manages a separate capability/Grant ID and preserves
existing inspector access. It is local-only and never granted to player or operator
targets. Current creation authority code verifies active creator/source-world status,
exact administrative branch and field scope, matching immutable authorization Event,
and absence of a later superseding grant/revoke. Old active projection restoration
cannot bypass a later sourced revoke. Rebuild/reopen restores the latest grant state.
This enables an authorization prerequisite only; the world-creation API/genesis
workflow is still under implementation. No target world, principal or token is
created by this provisioning command. Legacy omitted purpose keeps its prior hash.

Use the existing service's configured bearer mapping separately; do not pass secrets
on these CLI flags or store credentials in Studio metadata. The tool uses the existing
Store opener/migrations; as with any local administrative operation, target a known
database and have a backup before operating outside disposable development fixtures.

## Authority and recovery

`ConfigureStudioAccessLocal` uses the existing atomic private command owner:
explicit operator attribution, expected head, preserved request hash/key, one
`StudioAccessConfigured` Event and grant projection in the same transaction. It
does not advance world time or produce public Outbox/Knowledge. A scoped
runtime-diagnostic audit records the actual branch; the generic private command
helper no longer hardcodes its audit into the M2 world.

Active RP work and overdue scheduler boundaries must settle first. Inconsistent
world chronology is rejected. In particular, the legacy M1 fixture's January clock
predates its September genesis, so it is not a valid configuration target without
an independently designed timeline repair. This tool never silently retimes a world.
Actual M2 Agent/RP worlds with initialized clocks are supported; a cohort-only
bootstrap without a clock must finish its existing world preparation first.

Projection comparison/rebuild derives the latest per-target/capability grant from
the immutable Event sequence. Missing/corrupt grants are repaired, unsourced grants
for these two newly introduced capabilities are removed, revocations remain revoked.
Conflicting unsourced grants are removed before originals are restored. Fields,
subject, principal, capability, source Event and amount-limit corruption are compared.
No schema migration or second ACL store. Existing unrelated capabilities are untouched.

## Evidence / current limits

Initial attempts correctly failed on unsuitable fixtures: legacy M1 chronology,
then bare M2 bootstrap without a clock. Neither was accepted by weakening checks.
Fixtures now use actual initialized M2 Agent world. Read-only M1 grant-shape tests
retain explicit disposable grants and test reopen only; real source-backed
rebuild/revoke/corruption testing belongs to StudioAccess tests.

Focused90447 passed storage0.419s/adminCLI0.239s/HTTP0.189s. Expanded related
race/vet/regression is tracked in progress until terminal. Tests include atomic
injected failure, exact retry/mismatch, unsupported target/actor, grant loss/repair,
revocation/corruption/reopen, old grant retry without resurrection, conflicting
unsourced grant, missing DB refusal and absence of an HTTP provisioning route.
Latest current-source76540 filtered storage/admin race PASS4.804s/3.184s,
including rollback-head and old grant retry-after-revoke assertions. Broader25934
terminal PASS: storage608.488s/admin3.215s/HTTP58.981s, whole-backend vet and
subsequent full HTTP/server normal12.476s/0.165s. No backend checks remain live.

This is the setup prerequisite for the actual Studio UI, not completion of RP8.
World creation, pack installation, complete causal/knowledge Inspector and final
integration remain required.
