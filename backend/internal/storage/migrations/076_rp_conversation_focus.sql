-- Provenance for a pinned application activation preference, not world state.
-- Existing plans keep their original order/reason and acquire no inferred focus.
CREATE TABLE rp_turn_listener_activations_next (
  turn_run_id TEXT NOT NULL REFERENCES rp_turn_runs(turn_run_id),
  npc_entity_id TEXT NOT NULL REFERENCES agent_profiles(agent_id),
  disposition TEXT NOT NULL CHECK (disposition IN ('activated','externally_controlled','not_activated')),
  reason_code TEXT NOT NULL CHECK (reason_code IN ('legacy_compatible','direct_address','conversation_continuation','stable_fallback','external_controller','responder_limit')),
  activation_rank INTEGER CHECK (activation_rank IS NULL OR activation_rank >= 0),
  owner_source_event_id TEXT REFERENCES events(event_id),
  conversation_source_event_id TEXT REFERENCES events(event_id),
  PRIMARY KEY(turn_run_id,npc_entity_id),
  CHECK ((disposition='activated' AND activation_rank IS NOT NULL AND owner_source_event_id IS NULL)
    OR (disposition='externally_controlled' AND activation_rank IS NULL AND owner_source_event_id IS NOT NULL)
    OR (disposition='not_activated' AND activation_rank IS NULL AND owner_source_event_id IS NULL)),
  CHECK ((reason_code='conversation_continuation' AND disposition='activated' AND conversation_source_event_id IS NOT NULL)
    OR (reason_code<>'conversation_continuation' AND conversation_source_event_id IS NULL))
) STRICT;
INSERT INTO rp_turn_listener_activations_next
  (turn_run_id,npc_entity_id,disposition,reason_code,activation_rank,owner_source_event_id)
SELECT turn_run_id,npc_entity_id,disposition,reason_code,activation_rank,owner_source_event_id
FROM rp_turn_listener_activations;
DROP TABLE rp_turn_listener_activations;
ALTER TABLE rp_turn_listener_activations_next RENAME TO rp_turn_listener_activations;
CREATE UNIQUE INDEX ux_rp_turn_activation_rank
ON rp_turn_listener_activations(turn_run_id,activation_rank) WHERE activation_rank IS NOT NULL;
INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-conversation-focus-076-2026-09-30','2026-09-30T00:00:00Z');
