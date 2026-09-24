# RP-5 opportunity configuration

## Optional WARM simulation

At initial policy installation, `warm_enabled:true` enables low-frequency off-scene rule decisions (default false). This is not another model setting: at most4 named actors per Wait, selected by sourced important relationship or imminent scene appointment, and at most once per six world hours per actor across sessions. No provider is called for WARM. Actors may prepare for their own upcoming work using an immediately legal first route step, or wait; they do not learn the player's location, start work/pay early, or produce invented speech. Existing schedules and aggregate simulation continue. Private roster/reasons/effects are not broadcast as player knowledge; actual later co-location can be observed normally. The immutable policy cannot be reset to bypass cadence.

## Explicit social seeking while waiting

Existing authenticated `POST /api/v1/rp/actions/wait` accepts optional `opportunity_intent:"social"`; omit it for ordinary waiting. It applies only to friendly-contact sources already eligible in the controlled character's current scene. This adds half the configured contact base probability within the existing cap; quiet, recent density, major-change, busy and cooldown rules still apply. Zero base stays zero and seeking creates no friendship, speech, rare event or guarantee. Other intent strings are rejected; store/work/weather are not boosted by this social preference.

Intent is bound into the durable wait request hash. Same-key changes fail; restart/exact retry retain the original result. The first actor/source/hour receipt is reused even if a fresh-key wait changes the intent in that hour. Intent does not persist to future waits, does not create an in-world utterance/knowledge fact, and is stripped from public broadcasts/provider context. Existing empty-intent requests preserve their legacy hashes. Generic exploration/other intent categories are not implemented.

Play exposes “等待时，愿意和熟人聊聊” beside the wait actions, unchecked by default. It applies to the next submitted wait and resets after that wait completes and the refreshed observation is loaded. While an action is pending, the control is disabled; the exact submitted intent is retained in the existing local recovery bookmark, not a permanent preference. Refresh/restart does not replace the pending request with the current checkbox state. The explanation explicitly says no response is guaranteed and no speech is made on the player's behalf.

`POST /api/v1/opportunities/policy/define` uses existing bearer authentication and binds `binding.principal_id` to the authenticated principal (omit it or supply the same identity). Domain authorization requires the original branch's still-active creator construction authority; player/NPC control does not qualify.

Request:

```json
{
  "binding": {
    "instance_id": "inst_m2_t09",
    "branch_id": "br_main",
    "expected_head": 123,
    "idempotency_key": "install-contact-opportunities"
  },
  "policy": {
    "stream_seed": "explicit-run-stream",
    "contact_basis_points": 1500,
    "work_basis_points": 1000,
    "community_basis_points": 1000,
    "cooldown_hours": 6,
    "history_hours": 24
  }
}
```

Replace the example head with the actual current branch head; pending RP actions must finish first. Basis points range0–5000 (10000 means100%, which is not allowed). Cooldown1–168 hours; history is at least cooldown and at most720 hours. Stream seed is an explicit simulation identifier, not an API credential. It remains internal configuration and is not included in NPC context or public wait broadcasts.

One immutable installation per branch. Exact retries reauthorize and return the original `event_id`, sequence, world time, fact and `replayed:true`; changed same-key requests fail with idempotency mismatch, while another key cannot reset the installed stream. No edit/delete/reset endpoint exists. New worlds are not implicitly enabled. Executable sources include friend contact, own work-change reactions, separately configured local weather, storefront shortages and route works; remaining RP-5 source families are under development.

Optional `community_basis_points` also ranges0–5000 and defaults off. It enables reactions to an already-known effective local rule amendment/repeal, not omniscient news or automatic enactment. First actual hearing and effective time define source age; repeat announcements do not refresh it. Private draw receipts share contact/store/work cooldown and history; providers see only own known law and selection. See community-opportunities.md for effects and remaining coverage.

Optional `visit_basis_points` (0–5000) and `rare_visit_basis_points` (0–100) enable attempts to revisit own remembered public places or places where a trusted person was last actually encountered at least seven days ago. Both default off and must be chosen at initial installation; positive rare chance requires `history_hours`≥168. These are optional HOT movement opportunities, not guaranteed meetings or knowledge of another person's current position. One source/kind per actor/hour is pinned, even on a miss; no fallback reroll. Rare receives no quiet/seeking boost. Current work, shared contact/store/work/community/visit pressure and target-time traversability apply. See remembered-visits.md for full boundaries and verified effects.

Optional `work_basis_points` is0–5000, defaults0 (disabled), and must be chosen at initial policy installation. It selects opportunities to react to already-known Career changes: own accepted job, regularization, raise/position/exit notices, leave/overtime decisions and aggregate exit. It does not randomly grant/terminate employment, advance effective dates, pay wages, or expose private managerial assessments. Only the latest eligible own change in the history window is considered, with busy/upcoming-work suppression, major-change pressure and same-source repeat control. The same hourly draw is retained; once selected, the same source is suppressed throughout history. Selected context does not force speech; independent urgent goals retain precedence. Raw receipts/seed/chance remain private and are stripped from wait broadcasts. Old policies remain compatible and disabled for this family; no policy migration/reset endpoint is introduced.

After installation, the existing wait endpoint records private contact opportunity receipts from actual own-known friendly relationships. Existing initiative processing uses these receipts, with a missed optional contact committing quiet without a provider call; independent sourced needs retain existing processing. Wait response/public broadcast do not return raw receipts or stream details. Enabling configuration does not create friendship, wealth, speech or guaranteed events.

Contact, shortage and work-change offers share actor-level recent density and cooldown across sessions. Only selected offers count, once per draw identity and from their original evaluation time; choosing silence does not reset an offered opportunity. Within a new Wait evaluation, contact precedes shelves, then work; an offer selected earlier contributes pressure to later families. Previously recorded hourly draws remain unchanged. Another person's offers and shared weather/road conditions do not consume this actor's invitation budget. All three reaction families reduce extra opportunities after the actor's own recent job-loss notice; the notice is not early contract termination. This is suppression, not an event quota or a guaranteed event every N turns.

Quiet-history bonus: both policy installation and the actor's routine source must predate the full configured history window, with no selected offers, known work changes or supported major notices in that window. Eligible minor opportunities then gain `min(base/4,500)` basis points under the existing non-guaranteed ceiling. Disabled probability stays0; busy/cooldown suppression still wins. A new world does not inherit an assumed quiet past, and the same hourly draw is not recalculated when the window matures. This describes known source families, not detection of every possible major world change; explicit seeking preference is not yet implemented.

Validation: actual HTTP/store test covers missing authentication, NPC denial, principal spoofing, creator installation, mismatched/fresh-key reset denial and reopen retry. Detailed contact consequences/privacy/cooldown remain separately verified by storage integration tests, not yet a single end-to-end HTTP journey.

## Place weather source

`POST /api/v1/opportunities/environment/define` uses the same bearer/principal binding and creator authority. Install the branch policy first. Supply the same `binding` structure with the latest branch head and a distinct idempotency key; replace `policy` with:

```json
{
  "source": {
    "place_id": "place_m2_cafe",
    "rain_basis_points": 1500,
    "cooldown_hours": 6
  }
}
```

Use an actual active place in the branch. Rain probability0–5000 basis points; cooldown1–168 hours. One immutable source per place; retries reauthorize, changed same-key requests fail, and fresh keys cannot replace it. Returned provenance identifies the actual place, policy and creator source Events. Installation does not itself cause rain or grant anyone weather knowledge.

Existing completed waits materialize current-place weather on demand. Co-located observers share the first accepted condition in that UTC hour; repeated observation/restart cannot reroll it or extend its cooldown. Public RP observation exposes only current local condition and its occurrence provenance/interval through optional `environment`, never seed/roll/configuration. Missing or expired evidence is omitted, not interpreted as clear. Cautious NPCs with a sourced reachable home may voluntarily return during rain when urgent needs/work do not take precedence. No road closure or appointment cancellation is imposed.

HTTP configuration authority/recovery and actual stored movement are separately tested; no browser weather widget or all-HTTP movement journey is claimed.

## Public storefront and shortage reactions

`POST /api/v1/opportunities/storefront/define` uses the same authenticated creator binding. Supply the current `binding` plus:

```json
{
  "source": {
    "place_id": "place_m2_cafe",
    "store_actor_id": "actor_m2_food_store",
    "sku_id": "m2_staple",
    "notice_basis_points": 1500
  }
}
```

The store/SKU must already exist in this branch with actual conserved stock provenance; the place must be active. One immutable place per store/SKU and at most8 shelf entries per place. Exact retry reauthorizes; changed same-key requests and fresh-key duplicate declarations fail. This adapter currently binds existing M2 economic stores; it is not an individual RP purchase API or a generic store-creation endpoint.

Omitted/zero `notice_basis_points` publishes local shelf availability only, without requiring a random stream. Nonzero0–5000 notice probability requires the already installed branch opportunity policy and pins that policy Event; its history/cooldown apply to optional shortage reactions. The source cannot later be edited to enable reactions, so choose before installation. No stock quantity or balance is changed by declaration or draws.

Authorized observation/own NPC context includes optional local `stores` with store/SKU/place, availability and source Events, not exact stock/funds/supplier data. A real zero-stock condition may generate a private wait receipt for nearby NPCs. Same source/window reuses the accepted draw, including misses; selected offers suppress another reaction to that same stock-out throughout the configured history window. Other stock-out sources also respect actor cooldown and recent density. This counts offered opportunities, not guaranteed speech. Busy work suppresses chance; urgent needs and independent motivations remain valid. No provider is forced to mention a shortage.

Private receipts/seed/roll are omitted from public wait outbox and filtered NPC opportunity context. Repeated observation itself does not generate a notification or persistent Knowledge. Actual accepted speech uses existing hearing/Knowledge and recovery transactions. Tests separately cover HTTP authority/retry and the real stock-out→draw→deterministic response/hearing→restart/history journey; no browser purchase flow is claimed.

## Bounded route works / transit delay

`POST /api/v1/opportunities/transit/define` uses the same authenticated creator binding. This declares a condition-based scheduled source, not a random traffic roll or NPC permission to close roads. Request:

```json
{
  "binding": {
    "instance_id": "inst_m2_t09",
    "branch_id": "br_main",
    "expected_head": 123,
    "idempotency_key": "scheduled-route-works"
  },
  "from_place_id": "place_m2_cafe",
  "to_place_id": "place_m2_work_ada",
  "starts_at": "2026-09-24T08:00:00Z",
  "ends_at": "2026-09-24T09:00:00Z"
}
```

Replace the head and dates with current-world values. Both route directions and active endpoints must already exist. Times must be whole-second RFC3339; the interval starts strictly in the future, within30days, and lasts at most6hours. It covers both directions, is half-open `[start,end)`, and must leave at least one hour between works on the same undirected route. Reversing endpoints or using a fresh key cannot bypass overlap/cooldown. Exact retries reauthorize and replay the original declaration. There is no edit/cancel/delete route.

During the interval, direct player/NPC movement cannot pass an obstructed route unless a known open alternative permits immediate arrival. Scheduled movement checks the full declared path (including intermediate edges), records actual waiting, and queues a distinct retry instead of teleporting. It preserves the original appointment identity/time/definition and all existing economic ownership; no automatic wage penalty is invented. A later accepted scheduled activity can supersede an obsolete delayed arrival. Rebuild repairs transit-touched queue state from Events. Legacy schedules without a path in the declared RP topology retain legacy behavior; arbitrary world-pack topology completeness is not claimed.

`transit_works` in RP observation and own NPC context lists at most16 current adjacent works with route endpoint, expiry and source Event. Future, expired and distant works are omitted; this is not a global forecast or private appointment feed. An actor's own delayed `next_schedule` additionally includes `original_world_time` and `delay_source_event_id`, while `world_time` is the effective due time. Delay does not extend the original Career leave/overtime decision deadline.

Actual store tests cover delayed/repeated/cross-midnight arrival, projection damage/restart repair, player/NPC agreement, expiry, superseded appointments, late attendance and preserved existing wage policy, plus valid future leave/resignation. HTTP tests separately cover creator authority/spoofing/overlap denial/recovery. No browser traffic UI or all-HTTP full journey is claimed.
