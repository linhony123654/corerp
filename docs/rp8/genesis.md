# Source-backed Studio genesis

`Store.PrepareStudioWorld` is an internal delivery increment, not yet an HTTP/UI
creation workflow. It requires independent sourced `world.create` authority on an
existing administrative branch and a validated `core.StudioWorldSpec`.

One SQLite immediate transaction creates the target world/branch, paused clock,
pending-activation Rule Epoch, immutable `StudioWorldPrepared` Event, accepted
specification, existing Cohort owner, namespaced currency/SKU/accounts/stock locations,
balanced posted opening issuance, stock creation and population creation facts.
Zero opening money/stock emits no illegal zero posting/movement. Population and
resources are not duplicated into a parallel state store. A scoped private audit is
recorded; no player-facing Outbox message is published for this preparation.

Creation uses globally namespaced stable object IDs. Command identity comes from
principal, administrative source and request key; the full request hash includes
target and every specification field. Exact retries return the original receipt;
same key with changed settings/target fails, and a new key cannot overwrite an
existing target. Current creation permission is rechecked even for retries. Source
administrative world history/head/resources are unchanged by target creation.

The receipt's `prepared` status describes this genesis operation, not current live
world readiness. World and clock remain paused; the epoch lock explicitly says
`awaiting_package_activation`. Genesis alone creates no named participants.
Spatial links, active installed packages and player control must still complete
before activating and returning the world to Play. No migration required.

Verification uses actual two target worlds (funded and zero-resource), failure
injection/rollback, balanced journal, population source, unchanged administrative
head, projection corruption/repair, rebuild/reopen, retry and conflict behavior.
These tests establish genuine saved genesis, not a fully playable second world.

## Conserved participant preparation

`Store.PrepareStudioParticipants` accepts the exact saved genesis request. It
reauthorizes current source `world.create` before even returning a retry receipt;
role membership, a different creator, or altered settings do not authorize it.
First execution requires untouched paused genesis and initial Cohort resources
matching the immutable declaration.

The existing Cohort materialization owner now supports a caller-owned transaction.
The public method retains its original scoped-grant check and receipt format.
Studio binds its narrowly constructed commands to the saved genesis instead of
granting general materialization permission. Every declared person receives one
population unit and a deterministic integer share of remaining money and stock
(`remaining balance / remaining population`, in declared order). Undistributed
resources remain in the Cohort; the last person receives the remainder if the
entire population is named. No additional wealth or population is created.

All participants, accounts, balances, lineage, population/stock movements, posted
journals and existing materialization Outbox entries commit together. An error in
the second participant or at final commit rolls the whole participant set back,
leaving the previously saved genesis intact. Namespaced IDs separate worlds with
the same local person keys. Exact retry uses existing Cohort receipts; a partially
recorded set is rejected rather than silently completed.

These are conserved economic entities, not yet spatial actors or playable
characters. No player principal/control grant, place, link, package activation,
HTTP endpoint or UI has been added by this preparation step.

Verification: actual SQLite three-world participant test covers nondivisible,
zero and fully allocated resources, partial-write/final-commit fault injection,
unchanged source head, corrupt initial projection rejection, rebuild/compare,
reopen/exact retry and revoked-authority denial. Expanded storage race (including
legacy Cohort transitions, RP setup and post-term wage-claim materialization)
passed in 73.059s; wholebackend `go vet ./...` passed. This is increment evidence,
not the full RP8 stage gate.

## Spatial preparation

`Store.PrepareStudioSpatial` follows conserved participant preparation, saving
one `StudioSpatialPrepared` Event through the existing private fact transaction.
It freshly authorizes and matches the exact accepted genesis, requires the
expected participant head and initial paused clock, and verifies each individual
materialization in the target world. Shared saved-genesis authorization is also
used by participant preparation; no additional ACL or state owner is introduced.

The transaction inserts existing agent place definitions, bidirectional RP links,
distinct namespaced player/agent principals, spatial profiles, authoritative
initial movement records and their position projections. The Event references
the immutable genesis declaration. Roles are not control permissions: no control
grant or credentials are created, no package/epoch is activated, and no observation
or public Outbox entry implies that characters know the whole topology.
All of this rolls back on failure; exact retry remains authorized even after
later world events, while changed specifications or revoked authority are denied.

Current movement semantics matter: existing RP links express reachability and
MoveRP is immediate. The declaration's link `minutes` is saved but is not yet
consumed as production travel duration. Do not expose that setting as an effective
travel-time rule until its runtime consumer is implemented.

Actual SQLite tests prepare two worlds, verify namespaced actors/routes,
rollback and unchanged source head, corrupt/repair positions, rebuild and reopen.
A test-only schedule definition then drives the real scoped movement owner:
the first world's position, clock, co-location observations and Outbox advance,
the second and administrative worlds do not, and repeating the scheduler does
not duplicate the move. This proves scoped movement execution, not a production
schedule-authoring API, activated packages, player authorization or completed Play
creation journey.

Final focused race (spatial/participant/genesis and legacy RP participant setup)
passed in 10.358s, followed by wholebackend `go vet ./...`. Full RP8 acceptance
and creator-to-Play browser verification remain pending.
