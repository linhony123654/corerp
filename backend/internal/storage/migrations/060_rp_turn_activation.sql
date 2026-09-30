-- Durable application orchestration only. Hearing remains world evidence and
-- NPC effects remain canonical Events; these rows account for which heard
-- listeners were allowed to propose an effect for a specific turn.
ALTER TABLE rp_turn_runs ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'legacy'
  CHECK (execution_mode IN ('legacy','deterministic','orchestrated','multi_agent'));
ALTER TABLE rp_turn_runs ADD COLUMN responder_limit INTEGER NOT NULL DEFAULT 0
  CHECK (responder_limit BETWEEN 0 AND 8);

CREATE TABLE rp_turn_listener_activations (
  turn_run_id TEXT NOT NULL REFERENCES rp_turn_runs(turn_run_id),
  npc_entity_id TEXT NOT NULL,
  disposition TEXT NOT NULL CHECK (disposition IN ('activated','externally_controlled','not_activated')),
  reason_code TEXT NOT NULL CHECK (reason_code IN ('legacy_compatible','direct_address','stable_fallback','external_controller','responder_limit')),
  activation_rank INTEGER CHECK (activation_rank IS NULL OR activation_rank >= 0),
  owner_source_event_id TEXT REFERENCES events(event_id),
  PRIMARY KEY(turn_run_id,npc_entity_id),
  FOREIGN KEY(npc_entity_id) REFERENCES agent_profiles(agent_id),
  CHECK ((disposition='activated' AND activation_rank IS NOT NULL AND owner_source_event_id IS NULL)
    OR (disposition='externally_controlled' AND activation_rank IS NULL AND owner_source_event_id IS NOT NULL)
    OR (disposition='not_activated' AND activation_rank IS NULL AND owner_source_event_id IS NULL))
) STRICT;
CREATE UNIQUE INDEX ux_rp_turn_activation_rank
ON rp_turn_listener_activations(turn_run_id,activation_rank)
WHERE activation_rank IS NOT NULL;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-turn-activation-060-2026-09-27','2026-09-27T00:00:00Z');
