-- Application presentation configuration, not world/character authority.
CREATE TABLE rp_style_revisions (
  revision_id TEXT PRIMARY KEY,
  binding_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK (revision >= 1),
  principal_id TEXT NOT NULL REFERENCES principals(principal_id),
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  patch_json TEXT NOT NULL CHECK (json_valid(patch_json)),
  created_at_utc TEXT NOT NULL,
  UNIQUE (binding_id, revision),
  UNIQUE (principal_id, idempotency_key)
) STRICT;

CREATE TABLE rp_style_bindings (
  binding_id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  scope TEXT NOT NULL CHECK (scope IN ('world','session','scene')),
  session_id TEXT REFERENCES rp_sessions(session_id),
  place_id TEXT REFERENCES agent_places(place_id),
  revision_id TEXT NOT NULL REFERENCES rp_style_revisions(revision_id),
  FOREIGN KEY (instance_id,branch_id) REFERENCES branches(instance_id,branch_id),
  CHECK ((scope='world' AND session_id IS NULL AND place_id IS NULL)
    OR (scope='session' AND session_id IS NOT NULL AND place_id IS NULL)
    OR (scope='scene' AND session_id IS NOT NULL AND place_id IS NOT NULL))
) STRICT;
CREATE UNIQUE INDEX ux_rp_style_scope ON rp_style_bindings(instance_id,branch_id,scope,COALESCE(session_id,''),COALESCE(place_id,''));

CREATE TABLE rp_turn_styles (
  turn_run_id TEXT PRIMARY KEY REFERENCES rp_turn_runs(turn_run_id),
  profile_json TEXT NOT NULL CHECK (json_valid(profile_json)),
  sources_json TEXT NOT NULL CHECK (json_valid(sources_json))
) STRICT;
-- Legacy turns used a fixed second-person literal renderer. Preserve that
-- style even for interrupted legacy turns; completed narrative is untouched.
INSERT INTO rp_turn_styles(turn_run_id,profile_json,sources_json)
SELECT turn_run_id,'{"version":"corerp.style.v1","pov":"second_person","tense":"present","verbosity":"normal","dialogue_ratio":100,"description_density":0,"inner_monologue_policy":"none","prose_instructions":"","forbidden_patterns":[],"narrative_pack_ref":"builtin/plain@1"}','[]' FROM rp_turn_runs;

CREATE TRIGGER rp_style_revision_no_update BEFORE UPDATE ON rp_style_revisions
BEGIN SELECT RAISE(ABORT,'STYLE_REVISION_IMMUTABLE'); END;
CREATE TRIGGER rp_style_revision_no_delete BEFORE DELETE ON rp_style_revisions
BEGIN SELECT RAISE(ABORT,'STYLE_REVISION_IMMUTABLE'); END;
CREATE TRIGGER rp_turn_style_no_update BEFORE UPDATE ON rp_turn_styles
BEGIN SELECT RAISE(ABORT,'TURN_STYLE_IMMUTABLE'); END;
CREATE TRIGGER rp_turn_style_no_delete BEFORE DELETE ON rp_turn_styles
BEGIN SELECT RAISE(ABORT,'TURN_STYLE_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp2-styles-026-2026-09-23','2026-09-23T00:00:00Z');
