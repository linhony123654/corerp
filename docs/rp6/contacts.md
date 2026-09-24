# Known-contact view

`POST /api/v1/rp/contacts/read` accepts session_id and optional after_entity_id; the authenticated principal is bound by the HTTP layer. Storage rechecks session ownership, active control grant, active session and character binding on every read. No account, arbitrary observer or target-principal selector is exposed.

The view selects only the controlled character's own agent_presence/speaker_said knowledge, joins the source event and subject profile within the session's actual world/branch, and excludes self. Current materialized display name is public identity; latest learned_world_time is labeled “最近获知”, not a claim about another person's current activity or location. No relationship scores, goals, accounts, memories of other observers, remote positions or invented phone numbers are returned. Contacts may remain known after leaving the scene; opening this view does not create knowledge or events.

Responses contain contacts (entity_id/display_name/last_known_world_time), world_time, observation_cursor and optional next_after_entity_id. Stable exclusive entity-ID seek paging returns at most50 contacts; a51st row supplies the next cursor. Pages are individual current snapshots, not a retained global snapshot; refresh starts over and discovers contacts added before the current cursor. This is a read projection, not another address-book database.

PlayContacts uses the existing paper-sheet interaction: desktop centered, phone bottom-aligned,44px controls, keyboard boundary/Escape and focus return. Empty/loading/error states are explicit. Failed refresh retains old records with an old-snapshot warning; load-more merges identities without duplicates; no page cursor or credential is persisted.

## Evidence and limits

Actual storage test plays dialogue with Cai, travels and speaks with Ada, retains those known people and excludes off-scene unknown Bo; validates seek cursor/end, reopen, foreign session/revoked control and unchanged head. HTTP checks private session/auth/spoofing. Normal tests PASS storage0.338s/HTTP0.232s; frontend build/typecheck PASS. Dedicated real-service browser first reads0 contacts then3 after actual play/restart, matching SQLite own knowledge exactly; checks minimal field set, failure/retry, focus, viewport and no Event/knowledge/bookmark mutation. Artifacts /tmp/corerp-rp1-e2e-cSeAbv. Full regression/race results are in progress.

This completes the initial owner-backed contact reading path, not work notices, phone communications, map, arbitrary prose generation or100+turn acceptance. No contacts are inferred merely by inspecting the population table; the current scene may show a person before a durable knowledge record exists.
