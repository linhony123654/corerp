-- Event-rebuilt projections for the minimal scene-continuity contract.
-- Both tables are derived exclusively from committed events; rows are inserted
-- by commit paths and updated only by their terminal event. They hold no
-- independent authority.

-- Activity lifecycle: in_progress -> completed | cancelled | failed.
-- Terminal transitions must come from committed events; an activity without
-- a terminal event is genuinely still in progress, never implicitly done.
CREATE TABLE rp_activities (
  activity_id TEXT PRIMARY KEY,
  actor_id TEXT NOT NULL,
  place_id TEXT NOT NULL,
  activity_code TEXT NOT NULL,
  started_world_time TEXT NOT NULL,
  duration_minutes INTEGER NOT NULL CHECK (duration_minutes >= 1),
  status TEXT NOT NULL CHECK (status IN ('in_progress','completed','cancelled','failed')),
  start_event_id TEXT NOT NULL,
  end_event_id TEXT,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  FOREIGN KEY (actor_id) REFERENCES agent_profiles(agent_id),
  FOREIGN KEY (place_id) REFERENCES agent_places(place_id),
  FOREIGN KEY (start_event_id) REFERENCES events(event_id),
  FOREIGN KEY (end_event_id) REFERENCES events(event_id),
  FOREIGN KEY (instance_id,branch_id) REFERENCES branches(instance_id,branch_id)
) STRICT;
CREATE INDEX ix_rp_activities_actor_status ON rp_activities(actor_id, status);
CREATE INDEX ix_rp_activities_place ON rp_activities(instance_id, branch_id, place_id, status);

-- An actor's own committed words, movements and activities. agent_knowledge
-- cannot hold these (its schema forbids observer == subject); this log is the
-- autobiographical channel decision inputs read as own_actions.
CREATE TABLE rp_own_actions (
  agent_id TEXT NOT NULL,
  event_id TEXT NOT NULL,
  action TEXT NOT NULL,
  activity_code TEXT,
  text TEXT,
  place_id TEXT,
  world_time TEXT NOT NULL,
  status TEXT,
  instance_id TEXT NOT NULL,
  branch_id TEXT NOT NULL,
  last_event_sequence INTEGER NOT NULL CHECK (last_event_sequence >= 1),
  PRIMARY KEY (agent_id, event_id),
  FOREIGN KEY (agent_id) REFERENCES agent_profiles(agent_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id),
  FOREIGN KEY (instance_id,branch_id) REFERENCES branches(instance_id,branch_id)
) STRICT;
CREATE INDEX ix_rp_own_actions_agent_time ON rp_own_actions(agent_id, world_time);

-- Upgrade existing worlds: reconstruct the autobiographical rows that can
-- already be derived from pre-058 committed RP events.
INSERT INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence)
SELECT e.actor_id,e.event_id,'speech',NULL,json_extract(e.payload,'$.text'),json_extract(e.payload,'$.place_id'),e.world_time,NULL,e.instance_id,e.branch_id,e.event_sequence
FROM events e WHERE e.event_type='RPSpeechAccepted';
INSERT INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence)
SELECT e.actor_id,e.event_id,'leave',NULL,NULL,json_extract(e.payload,'$.to_place_id'),e.world_time,NULL,e.instance_id,e.branch_id,e.event_sequence
FROM events e WHERE e.event_type='RPNPCMoved';
INSERT INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence)
SELECT e.actor_id,e.event_id,json_extract(e.payload,'$.action'),NULL,NULL,COALESCE(json_extract(e.payload,'$.from_place_id'),json_extract(e.payload,'$.place_id')),e.world_time,NULL,e.instance_id,e.branch_id,e.event_sequence
FROM events e WHERE e.event_type='RPNPCDecisionRecorded' AND json_extract(e.payload,'$.action') IN ('silence','wait');

-- Widen the NPC decision vocabulary with act, and stop requiring a parent
-- utterance: initiative decisions have no speech turn, so the FK cannot hold
-- once their audit rows land here too. Rebuild preserves existing rows.
CREATE TABLE rp_npc_decisions_new (
  decision_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  parent_turn_id TEXT NOT NULL,
  npc_entity_id TEXT NOT NULL,
  event_id TEXT NOT NULL UNIQUE,
  action TEXT NOT NULL CHECK (action IN ('respond', 'refuse', 'silence', 'wait', 'leave', 'act')),
  input_hash TEXT NOT NULL,
  proposal_hash TEXT NOT NULL,
  proposal_json TEXT NOT NULL CHECK (json_valid(proposal_json)),
  UNIQUE (session_id, parent_turn_id, npc_entity_id),
  FOREIGN KEY (session_id) REFERENCES rp_sessions(session_id),
  FOREIGN KEY (event_id) REFERENCES events(event_id)
) STRICT;
INSERT INTO rp_npc_decisions_new SELECT * FROM rp_npc_decisions;
DROP TABLE rp_npc_decisions;
ALTER TABLE rp_npc_decisions_new RENAME TO rp_npc_decisions;
CREATE TRIGGER rp_npc_decisions_no_update BEFORE UPDATE ON rp_npc_decisions
BEGIN SELECT RAISE(ABORT,'NPC_DECISION_IMMUTABLE'); END;
CREATE TRIGGER rp_npc_decisions_no_delete BEFORE DELETE ON rp_npc_decisions
BEGIN SELECT RAISE(ABORT,'NPC_DECISION_IMMUTABLE'); END;

INSERT INTO schema_meta(schema_version,applied_at_utc)
VALUES ('corerp-rp-activity-continuity-058-2026-09-27','2026-09-27T00:00:00Z');
