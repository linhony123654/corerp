# RP6 local map

Play now has an on-demand **地图** entry alongside the existing quick-travel control. `PlayMap.vue` draws the actual current place as the root of a connection diagram and lists its adjacent destinations. It explicitly is not a geographic map: no invented direction, distance, coordinates, global place directory or remote-person tracking.

## Authority and interaction

The view reuses authenticated `POST /api/v1/rp/observe`. `reachable_places` remains the existing topological list and gains additive `can_move_now`. The storage observer calls the same `rpTransitAllowsImmediate` used by the player move owner inside its current snapshot, after closing the adjacency rows. This matters because direct roadworks may have an alternative open route; neither adjacency alone nor a local construction notice proves immediate availability.

The diagram uses existing `transit_works` for current adjacent notices only. That list is bounded to16 by its existing owner; it is not a complete future construction forecast. Availability uses the underlying movement authority, not that truncated notice list. No new projection, migration, physics, travel duration or world mutation is added. Observe retains its existing application-session observation-cursor update; it does not append Events, move anyone or advance the clock.

Every opening/refresh reads a fresh observation. Loading/errors are independent from world-action recovery; refresh failure keeps a labeled old diagram but disables departure. Close/Escape and keyboard containment are supported, and a response arriving after close cannot update the unmounted map.

Selecting a route sends its displayed origin/destination/**observation cursor** to the existing `act('actions/move')` flow. Durable original key and request are saved before dispatch, and the server rechecks actual position, current cursor and transit conditions. The diagram itself never assigns location. Lost responses therefore use the same committed-command replay as ordinary travel. Map reads do not overwrite the saved world-action bookmark or expose history/debug fields in the diagram.

## Evidence

- `go test ./internal/storage ./internal/transport/httpapi -run '^Test(RPTransitPerception|RPMove|RPSession|RPObserve)' -count=1 -timeout=5m`: PASS storage0.997s/HTTP0.227s. Current-local transit test now checks the actual route before, during and after source-defined works, including an unaffected alternative destination. Existing movement/session tests cover authorization and command behavior.
- Same selection with `-race`: PASS storage8.150s/HTTP2.691s. Storage/HTTP `go vet`: PASS. Full HTTP package regression: PASS10.779s.
- `npm run build`: PASS (typecheck + production bundle).
- `node scripts/verify-rp1-play.mjs --map --work --contacts --wallet --style-ui --regenerate --turn-stream --context-budget --fake-model`: PASS, `/tmp/corerp-rp1-e2e-ZiSwin`,33local HTTP decision fixture calls, not a live model. Real SQLite adjacency, actual source-defined works → UI waiting → blocked route → expiry → map travel, lost committed move response and exact request replay across server/page restart without a second movement. Includes previous RP6 increment/recovery checks, no credential persistence, no page errors or horizontal overflow.
- Actual mobile blocked-route and desktop entry screenshots inspected (first UI-identical run `/tmp/corerp-rp1-e2e-S7Jp64`). Ruled branches/current-place seal preserve the paper reading identity; blocked state is text as well as styling; mobile footer remains reachable by internal scrolling. The first run's later selector failure was a test assumption about the place name, corrected to read the fixture's actual name, not a product workaround.

This delivers the RP6 map entry, not full-stage acceptance. Messages, actual custom narrative execution,100+turn multi-day play and full-stage gates remain open. No stage checkpoint or deployment yet.
