-- Versioned presentation artifacts. World Events and canonical turn narration
-- remain authoritative; selected render is application state scoped by turn.
CREATE TABLE rp_narrative_renders (
  render_id TEXT PRIMARY KEY,
  turn_run_id TEXT NOT NULL REFERENCES rp_turn_runs(turn_run_id),
  style_json TEXT NOT NULL CHECK (json_valid(style_json) AND json_type(style_json)='object'),
  source_event_ids_json TEXT NOT NULL CHECK (json_valid(source_event_ids_json) AND json_type(source_event_ids_json)='array'),
  lines_json TEXT NOT NULL CHECK (json_valid(lines_json) AND json_type(lines_json)='array'),
  provider_kind TEXT NOT NULL,
  model_id TEXT NOT NULL DEFAULT '',
  created_at_utc TEXT NOT NULL
) STRICT;
CREATE INDEX ix_rp_narrative_renders_turn ON rp_narrative_renders(turn_run_id,created_at_utc,render_id);
CREATE TRIGGER rp_narrative_renders_no_update BEFORE UPDATE ON rp_narrative_renders
BEGIN SELECT RAISE(ABORT,'narrative render is immutable'); END;
CREATE TRIGGER rp_narrative_renders_no_delete BEFORE DELETE ON rp_narrative_renders
BEGIN SELECT RAISE(ABORT,'narrative render is immutable'); END;

CREATE TABLE rp_narrative_selections (
  turn_run_id TEXT PRIMARY KEY REFERENCES rp_turn_runs(turn_run_id),
  render_id TEXT NOT NULL REFERENCES rp_narrative_renders(render_id),
  selected_at_utc TEXT NOT NULL
) STRICT;
CREATE TRIGGER rp_narrative_selection_scope_insert BEFORE INSERT ON rp_narrative_selections
WHEN NOT EXISTS (SELECT 1 FROM rp_narrative_renders r WHERE r.render_id=NEW.render_id AND r.turn_run_id=NEW.turn_run_id)
BEGIN SELECT RAISE(ABORT,'narrative render belongs to another turn'); END;
CREATE TRIGGER rp_narrative_selection_scope_update BEFORE UPDATE ON rp_narrative_selections
WHEN NOT EXISTS (SELECT 1 FROM rp_narrative_renders r WHERE r.render_id=NEW.render_id AND r.turn_run_id=NEW.turn_run_id)
BEGIN SELECT RAISE(ABORT,'narrative render belongs to another turn'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-narrative-renders-073-2026-09-28','2026-09-28T00:00:00Z');
