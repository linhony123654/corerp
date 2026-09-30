-- The first completed, no-override presentation becomes the durable player-facing
-- text for a settled turn. World facts and NPC decisions remain in their own
-- immutable Event-backed stores; explicit regeneration stays presentation-only.
ALTER TABLE rp_turn_runs ADD COLUMN narrative_presentation_mode TEXT NOT NULL DEFAULT 'base'
  CHECK (narrative_presentation_mode IN ('base','deterministic','style_planner','full_prose','custom'));
ALTER TABLE rp_turn_runs ADD COLUMN narrative_presented_at_utc TEXT;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-rp-official-narrative-064-2026-09-27', '2026-09-27T00:00:00Z');
