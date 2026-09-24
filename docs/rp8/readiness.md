# Saved Studio world and separate player authority

`Store.SaveStudioWorld` is a local readiness transition, not external publication.
It accepts the exact original genesis, current creator binding and an explicit
existing active player principal. It does not mint/expose credentials, reinterpret
a creator/ops principal as a player, or reuse the creator token for Play.
The player logs in with its separately configured credential.

Before a first save, the world must still be initially paused with a validated
active System/Narrative package lock and the declared player must be a real
conserved spatial individual. One private Event transaction records
`StudioWorldReady`, the exact player-control grant, active world lifecycle and
ready clock. Failure rolls all of them back. The receipt describes that save
operation, not a snapshot of the later live world. Exact retries reauthorize
creation authority and return the original receipt after later actions/reopen.

The spatial profile's principal and the selected controlling principal are
distinct concepts: the existing exact `world.rp.control` grant decides control.
Selecting an existing player's identity permits that player's existing credential
to discover both old and new characters without duplicating accounts or storing
secrets in the world. Creator authority alone never satisfies RP control.

## Authorization, writes and recovery

Studio discovery and RP control share a predicate requiring active world
lifecycle plus the grant's exact matching ready Event, player, entity, fields and
source world. A raw grant pointing at genesis or another Event cannot reveal or
control a Studio character. Legacy non-Studio grants retain existing behavior.

Schema030 guards ready Events against UPDATE/DELETE/REPLACE; see
[migration SQL](schema-030-world-ready.sql). Existing comparison/rebuild restores
the ready grant/lifecycle from this source without creating a second ACL store.
It does not reactivate disabled principals. This first-save owner does not yet
provide a later rebind/revoke/archive workflow; any such future change needs its
own sourced transition rather than raw projection edits.

After a world is saved, the existing chronology boundary for external commands
and scoped scheduler mutation boundary verify current package pins. Unavailable
or inconsistent rules reject new writes; historical replay still reads committed
facts without those runtime consumers. Exact action receipts keep their existing
retry behavior, and package/ready recovery is handled by existing projection
rebuild.

## Wait and actual action evidence

Wait now invokes the scheduler for the session's actual world/branch. A foreign
world must be a saved controlled Studio world with validated active packages,
not merely any world with a manually inserted grant. The current System Pack
enables the movement phase only: other due phases are rejected **before** creating
a wait intent. Existing M2 dispatch remains unchanged. Broader career/economy
scheduling is not claimed from this change.

Actual SQLite tests create and save two independent worlds, then use the selected
player's real session APIs for Observe → Speak → Observe → Move → Observe → Wait.
They refresh the existing observation cursor contract rather than bypassing it.
The original administrative/demo world head remains unchanged. Tests also cover
prepared-world/creator denial, save rollback, scoped discovery, exact Wait retry,
unsupported-phase denial without a stranded intent, rule-corruption write refusal
with historical replay preserved, grant/epoch rebuild, reopen/save retry and
revoked creation authority.

This proves the backend saved-world/action path. Creator API/UI, default Play
binding selection, complete browser/restart journey, link-minute execution and
remaining cross-world career integration are still pending. A saved backend world
is not evidence that the original full RP8C user journey has passed.

Final selected Studio/Wait/discovery/session/speech/chronology/legacy-upgrade
storage race passed67.410s; wholebackend `go vet ./...` passed. No UI or
browser verification is claimed by these backend tests.
