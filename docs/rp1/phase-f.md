# RP-1F — Minimal CoreRP Play

Status: PASS locally, 2026-09-23. No production deployment.

## Implemented

- `src/components/PlayWorkspace.vue`: mobile-first player-only entry, credential form, durable session bookmark, current place/world time/actual presence, committed dialogue/action history, speech input, legal travel, scheduler wait, loading/error/empty/pending-retry states. Credentials stay in memory; original requests persist before sending. After a failed response, retry uses the same key and loads authoritative history before clearing pending intent.
- Default `/` uses Play; historical fixture and Inspector require explicit `/demo`. The Play flow never calls a creator endpoint or exposes NPC assets, goals, Knowledge internals, Event Ledger or projections.
- Observation adds topology-derived `reachable_places` and a bounded, session-scoped recent history view over settled turns and committed move/wait Events. No new migration, character authority, clock or location copy.
- Same-origin Vite `/api` proxy, updated page metadata, [local setup/recovery instructions](play.md), reproducible `npm run verify:rp1-play` using real Go processes, SQLite and Chromium.

## Visual contract and review

The web-inspiration-toolkit guided a quiet living-journal direction: the current place and dialogue are the focal point; warm paper, serif reading text, restrained green actions and an understated “此刻” stamp provide identity. Scene/presence and action controls support reading without dashboard cards. Motion is a brief entry fade with reduced-motion support; desktop uses a readable central column, mobile compresses metadata and leaves full-width actions/composer. No sourced visual assets or added runtime libraries.

References inspected: [80 Days](https://www.inklestudios.com/80days/) for place/travel-led narrative hierarchy; [Fallen London](https://www.fallenlondon.com/) could expose only its JS loading shell, so no visual claims or borrowed details were based on it. No reference assets were copied. Product requirements and existing serif tokens determined the implementation.

Inspected actual 390×844 and 1440×1000 renders. Revision reduced paper texture, formatted wait times, and corrected end scrolling so the last response sits above the composer. Final screenshot and browser assertion confirm no hidden latest reply or horizontal overflow. Semantic labels, visible focus, 44px action targets and reduced motion are implemented. Internal review: hierarchy/composition/typography/utility/responsive/polish 4/5; distinctness/richness/motion 3/5; no 1–2 blocker. No claim of an independent accessibility audit.

## Verification

- Full Go suite PASS: storage 56.532s, HTTP 4.924s, all other packages pass.
- `go vet ./...` PASS.
- Related RP/migration/HTTP/CLI race PASS: storage 63.040s, HTTP 11.685s, CLI 4.876s.
- New `TestRPPlayObservationRestoresScopedHistoryAndLegalDestinations` PASS: real speech/move/wait, DB reopen, transcript order, current legal routes, cross-session history exclusion and unauthorized reads.
- `npm run build` PASS (TypeScript check + production Vite build). No configured lint/unit-test script; meaningful browser verification added instead.
- M0 52 static checks, M1 evidence and M2 26 executable evidence references PASS.
- Real browser/process recovery PASS: initial Cai refusal and hearing; Ada home excludes Cai; Ada work schedule causes refusal; wait crosses actual morning/noon work and presence changes; cafe then contains all three NPCs. Drop the response after committed speech, close browser and HTTP process, restart with same DB and browser bookmark: original session and history return. Event/observation/utterance counts remain unchanged across recovery. Continue with another spoken turn. No browser exceptions and no credential persisted in browser storage.
- Representative artifacts: `/tmp/corerp-rp1-e2e-WZFoxm` (disposable database and mobile/desktop screenshots), recovery counts `32:30:8`. The executable script is durable evidence and prints a new artifact directory each run.

## Scope / recovery

E checkpoint: `98de398`. F checkpoint: the commit containing this report (`git log -- docs/rp1/phase-f.md`). All authority data is recoverable from the unchanged backend lineage. No new migration in F. Production identity/TLS, real LLM, relationship/episodic-memory engines, advanced narrative style, history pagination, browser bookmark export and multiplayer remain outside RP-1. Existing participant-scoped Outbox metadata still requires an external publisher to enforce audience routing; Play does not subscribe to an unfiltered event stream.
