# Creator and Play UI contract

Route: explicit `/studio/create`, linked from the existing inspector. Play remains
the default entry. Reuse the dark ink/amber/teal/Plex Studio identity, with a
numbered setup ledger and a narrow summary rail, not dashboard cards. The primary
subject is the world being specified; the final saved Event receipt is the only
success signal. Mobile puts the summary after the form; controls remain full-width,
44px minimum and keyboard reachable. No imagery, animation, remote fonts or new
dependencies. Play retains its separate paper/serif reading identity.

Form groups use short legends and independently clear labels, following the
[W3C WAI grouping-controls guidance](https://www.w3.org/WAI/tutorials/forms/grouping/)
inspected 2026-09-24. This is an interaction/semantic reference, not a visual skin.

Acceptance for this slice:

- Explicit source creation authority and existing target player identity; never
  derive identity from token text or return/generate a player credential.
- World name/resources/people/places and declarative System/Narrative selection;
  custom package JSON remains bounded and server validated. No executable plugin
  claim; links express reachability, not consumed travel durations.
- Before sending, freeze the complete request and idempotency key. Explicit local
  recovery consent; only configuration/IDs retained, never bearer credentials.
  Failed or abandoned requests remain separately recoverable. Reload/restart retries
  the same body. A cached request is not evidence that a world is ready.
- Ready receipt must match the request before offering Play. Separate player login
  uses the actual authorized discovery endpoint; no authority in navigation URLs.
- Play offers authorized bindings, pagination/empty/error/loading states and frozen
  session-open retries. Switching archives per-binding session bookmarks; no switch
  during pending actions/style writes/open requests. Legacy bookmarks are enriched
  after successful resume; they must resume once before switching from the UI.

Verification is real loopback Runtime/Vue/SQLite/browser, deterministic providers,
build/typecheck and responsive screenshots. Full RP8/Final gates remain separate.

## Implemented surface and limits

`StudioCreate.vue` uses `studioCreate.ts` to construct the initial two-person,
two-place world and canonical SHA-256-hashed declarative packages. Resource fields
are nonnegative safe integers, population 2–1,000,000, NPC budget 1–64. Server
validation remains authoritative, including exact imported package hashes,
dependencies, installed kinds and source/player permissions. No new schema or
frontend dependencies. Larger topologies remain available through the bounded API,
not a topology editor in this first form. Runtime link duration is not exposed as
an effective setting; hot upgrades and arbitrary executable plugins are unsupported.

Recovery uses explicit consent and per-request `corerp.studio.create.request.*`
local-storage records. The separate current pointer selects one immutable request;
starting another draft does not delete it. Local records are not encrypted: world
configuration and principal IDs remain on that browser, so use a trusted profile.
Clearing browser site data removes local recovery records, not worlds in the
Runtime. The UI currently provides no local-record deletion/export workflow.
No automatic browser storage of either creator or player bearer credentials.
Only a matching authenticated ready receipt is shown as saved; reload/credential
clear removes this success state and an exact retry must confirm it again.

The Play picker uses real `rp/bindings/list` pagination (20/page), explicit empty
and error states, automatic entry only when exactly one binding is available,
and current authorization checks on open/resume. It retains same-key open intents
across lost responses, and archives session cursors/pending intents per binding;
these records remain navigation/recovery hints, not world authority. Pending
actions/style changes/open requests must be resolved before switching worlds.

## Browser verification

Final67503 PASS `/tmp/corerp-rp8-play-worlds-1ccg2a`, terminal exit0. Covers all
listed creation/Play actions and additional delayed real response after credential
clear, tampered imported package refusal with unchanged world/Event/grant counts,
original request recovery after another draft, and legacy bookmark enrichment when
returning from Studio. First boundary run29755 found credential clear disabled
while saving; fixed actual UI and reran. Final2361 typecheck/build PASS2.54s;
original Play72055 PASS `/tmp/corerp-rp1-e2e-W05tp5`. All handles terminal.
Backend source/schema unchanged in this increment; previous backend race/vet
evidence remains in creator-api.md. Full backend/stage/Final gates still required.

`node scripts/verify-rp8-play-worlds.mjs --creator-ui` builds disposable local
Runtime/setup/admin binaries, grants actual source-backed creation authority,
uses real HTTP and SQLite, and operates Vue with Playwright. Loopback4408/4409,
deterministic providers, no real accounts or external model calls. Omitting the
flag creates the fixture through the API and directly verifies the Play picker.
All owned processes are stopped in the test's finally block.

Initial build65623 and original Play98431 PASS. API/picker80579 PASS with artifact
`/tmp/corerp-rp8-play-worlds-SWz6K9`. Creator98580 FAIL on the exact accessible name
of a wrapped select: its options polluted the name; explicit label references
fixed it. Creator21217 PASS `/tmp/corerp-rp8-play-worlds-8yWnG4`, including actual
generated packages and lost creation-response/restart recovery. Expanded49999 FAIL
because the test omitted the existing destination button's arrow suffix; selector
corrected without changing move behavior. Expanded28129 PASS
`/tmp/corerp-rp8-play-worlds-LYBEnm`: real NPC effects, installed terse style,
move/wait and unchanged administrative-world head, switching/reload/restart.
Inspector regression29201 PASS `/tmp/corerp-rp8-studio-mqLcdd`.

Screenshots reviewed at1440 and390 widths: initial Play picker heading inherited
dark-theme white, now uses Play text color. Restored creator forms showed default
draft inputs beside frozen originals; replaced that misleading state with the
frozen request summary only. Final reviewed ready/mobile and picker/mobile have
wrapped IDs, no horizontal overflow and readable action/receipt hierarchy.
This is not a full keyboard-only/accessibility certification or live-model test.
