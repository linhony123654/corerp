-- Presentation-only audit trail for narrative fallback. Never world authority:
-- it records why the displayed narrative came from the deterministic renderer
-- instead of the requested provider, so a silent fallback is impossible.
-- Populated exclusively by the narrative read path; reads never consult it
-- to decide facts.
ALTER TABLE rp_turn_runs ADD COLUMN narrative_fallback TEXT;

INSERT INTO schema_meta(schema_version, applied_at_utc)
VALUES ('corerp-rp-narrative-fallback-059-2026-09-27', '2026-09-27T00:00:00Z');
