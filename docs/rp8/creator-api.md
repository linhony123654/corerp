# Recoverable creator API

`POST /api/v1/studio/worlds/create` is authenticated and uses the existing
bounded strict JSON decoder and no-store response policy. The bearer identity
supplies `principal_id`; an explicitly different body principal is rejected.
The existing local administrator must have provisioned independent sourced
`world.create` authority. Inspector access, ops role and player tokens are not
creation authority; no public provisioning endpoint has been added.

Request fields:

|Field|Meaning|
|---|---|
|principal_id|Optional authenticated principal echo; cannot impersonate another caller|
|authority_instance_id / authority_branch_id|Existing administrative scope holding the creator's explicit grant|
|instance_id / idempotency_key|New target identity and stable creation request key|
|spec|Validated [world declaration](genesis.md)|
|system_package / narrative_package|Full validated [declarative bundles](packages.md)|
|player_principal_id|Explicit existing active player; its separate Play credential controls the saved character|

Unknown fields (including nested content/code) and invalid packages are rejected.
The full selected dependency set, package kinds, world settings and target player
are preflighted before creation. Required dependencies determine install order.

## Stages and retry contract

The coordinator invokes existing owners in order:
genesis → conserved participants → spatial preparation → two package installs →
activation → ready save. It does not hold a database transaction across the entire
workflow and does not invent a second workflow/state database.

An optional `creation_plan_hash` in the internal genesis request pins the full
coordinator request, including both packages and target player. Its omission
preserves previous standalone genesis request hashes. HTTP callers do not supply
this internal field. Deterministic child keys and expected heads reuse each
owner's exact receipt; a changed plan under the same request cannot alter a
partially created world.

On interruption, keep the **entire original request** and resend it unchanged.
Already committed stages replay; the first unfinished stage resumes. Before the
last stage there is no player grant and the world stays paused. Do not replace
the request body with just a target ID or invent a new key while recovering the
same creation. Current source authority is checked again; revocation can stop
continuation. This is recoverable staged save, not all-or-nothing multi-stage save.

First completed creation returns201; exact completed retry returns200. The receipt
contains instance_id, branch_id, entity_id, player_principal_id, ready_event_id,
event_sequence, status=`ready` and replayed. The status is historical operation
success, not a live world snapshot. No token or credential is returned.

## Verified path and pending UI

Storage tests inject failure at all seven commit stages, reopen the database and
resume the exact request; they also cover whole-plan mismatch after partial
creation, no early control grant, reversed required dependency order and
concurrent exact callers.

Actual authenticated HTTP tests create a world, discover/open it with the player's
separate token, run a complete deterministic player→NPC→narrative turn with the
installed style, and reopen the Runtime/store to verify original save and session
history. The remaining M2-only turn-intent guard was changed to additionally admit
authorized Studio worlds with validated active packages, not arbitrary worlds.

Creator UI, default Play's binding picker, browser restart/recovery and full RP8C
acceptance remain pending. Link-minute execution and broader career dispatch
limitations remain tracked; a working HTTP path is not a completed user journey.

Final selected race passed: storage82.133s and HTTP8.142s; wholebackend
`go vet ./...` passed. Existing turn regressions ran alongside the new tests.
