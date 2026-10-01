-- Frozen public input and finite plan, never private decision data. Legacy/v1
-- rows have no artifact and are not retroactively relabeled as v2.
ALTER TABLE rp_turn_runs ADD COLUMN narrative_artifact_json TEXT NOT NULL DEFAULT '{}'
  CHECK (json_valid(narrative_artifact_json) AND json_type(narrative_artifact_json)='object');
ALTER TABLE rp_narrative_renders ADD COLUMN artifact_json TEXT NOT NULL DEFAULT '{}'
  CHECK (json_valid(artifact_json) AND json_type(artifact_json)='object');

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-narrative-artifacts-078-2026-10-01','2026-10-01T00:00:00Z');
