package storage

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"corerp.local/backend/internal/core"
)

// Activity lifecycle payloads. Started carries the rule-bound duration;
// ended carries the terminal outcome. Models start activities; only the
// deterministic sweeper ends them.
type rpActivityStartedEvent struct {
	ActivityID       string `json:"activity_id"`
	ActorID          string `json:"actor_id"`
	ToPlaceID        string `json:"to_place_id"` // replay's position vocabulary: activities pin the actor's place
	ActivityCode     string `json:"activity_code"`
	StartedWorldTime string `json:"started_world_time"`
	DurationMinutes  int    `json:"duration_minutes"`
}

type rpActivityEndedEvent struct {
	ActivityID       string `json:"activity_id"`
	ActorID          string `json:"actor_id"`
	PlaceID          string `json:"place_id"`
	ActivityCode     string `json:"activity_code"`
	StartedWorldTime string `json:"started_world_time"`
	EndedWorldTime   string `json:"ended_world_time"`
	Outcome          string `json:"outcome"`
}

// readRPActivityRules returns the declared activity vocabulary with its
// rule-bound durations, or nil for worlds without active studio packages
// (demo worlds have no legal act vocabulary).
func readRPActivityRules(ctx context.Context, conn *sql.Conn, instance, branch string) (map[string]int, error) {
	packages, err := readStudioActivePackages(ctx, conn, instance, branch)
	if err != nil || packages == nil || packages.System.Content.SystemRules == nil {
		return nil, err
	}
	rules := packages.System.Content.SystemRules.Activities
	if len(rules) == 0 {
		return nil, nil
	}
	out := make(map[string]int, len(rules))
	for code, rule := range rules {
		out[code] = rule.DurationMinutes
	}
	return out, nil
}

// settleRPActivities deterministically terminates due activities in its own
// transaction: completion when world time has passed start+duration while the
// actor is still on scene, cancellation when the actor has left the place.
// It runs at RP turn open and after wait completion; a missed sweep is caught
// by the next one, so lag never becomes divergence.
func (s *Store) settleRPActivities(ctx context.Context, instance, branch string) error {
	return s.settleRPActivitiesWhen(ctx, instance, branch, nil)
}

func (s *Store) settleRPActivitiesWhen(ctx context.Context, instance, branch string, check func(*sql.Conn) (bool, error)) error {
	return s.settleRPActivitiesWhenFinal(ctx, instance, branch, check, nil)
}

func (s *Store) settleRPActivitiesWhenFinal(ctx context.Context, instance, branch string, check func(*sql.Conn) (bool, error), finish func(*sql.Conn, int64, int64) error) error {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "begin RP activity settle", err)
	}
	defer tx.Rollback(ctx)
	if check != nil {
		proceed, err := check(tx.conn)
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}
	}
	var head int64
	var worldTime string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence, c.current_world_time FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE b.instance_id=? AND b.branch_id=?`, instance, branch).Scan(&head, &worldTime); err != nil {
		return classifyMissing(err, "RP activity settle clock")
	}
	available, err := rpActivityContinuityTableAvailable(ctx, tx.conn, "rp_activities")
	if err != nil {
		return err
	}
	if !available {
		if finish == nil {
			return nil
		}
		if err := finish(tx.conn, 0, 0); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	type due struct {
		id, actor, place, code, started string
		duration                        int
		outcome                         string
	}
	var dues []due
	rows, err := tx.conn.QueryContext(ctx, `SELECT a.activity_id,a.actor_id,a.place_id,a.activity_code,a.started_world_time,a.duration_minutes,p.place_id FROM rp_activities a JOIN agent_positions p ON p.agent_id=a.actor_id WHERE a.instance_id=? AND a.branch_id=? AND a.status='in_progress' ORDER BY a.started_world_time, a.activity_id`, instance, branch)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "list RP activities to settle", err)
	}
	for rows.Next() {
		var d due
		var actorPlace string
		if err := rows.Scan(&d.id, &d.actor, &d.place, &d.code, &d.started, &d.duration, &actorPlace); err != nil {
			rows.Close()
			return core.WrapError(core.CodeStorageFailure, "scan RP activity to settle", err)
		}
		if actorPlace != d.place {
			d.outcome = "cancelled"
			dues = append(dues, d)
			continue
		}
		start, err := time.Parse(time.RFC3339, d.started)
		if err != nil {
			rows.Close()
			return core.NewError(core.CodeProjectionDiverged, "invalid RP activity start time")
		}
		now, err := time.Parse(time.RFC3339, worldTime)
		if err != nil {
			rows.Close()
			return core.NewError(core.CodeProjectionDiverged, "invalid RP world time")
		}
		if !now.Before(start.Add(time.Duration(d.duration) * time.Minute)) {
			d.outcome = "completed"
			dues = append(dues, d)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate RP activities to settle", err)
	}
	rows.Close()
	if len(dues) == 0 {
		if finish == nil {
			return nil
		}
		if err := finish(tx.conn, 0, 0); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	ids := make([]string, 0, len(dues))
	for _, d := range dues {
		ids = append(ids, d.id+":"+d.outcome)
	}
	sort.Strings(ids)
	key, err := core.HashJSON([]string{"rp_activity_settle", instance, branch, worldTime, ids[0], ids[len(ids)-1]})
	if err != nil {
		return err
	}
	suffix := key[7:]
	commandID, attemptID, batchID := "cmd_rp_activity_settle_"+suffix, "attempt_rp_activity_settle_"+suffix, "batch_rp_activity_settle_"+suffix
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, instance, branch, sequence, sequence).Scan(&epochID); err != nil {
		return classifyMissing(err, "RP activity settle Rule Epoch")
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	type committed struct {
		due      due
		eventID  string
		sequence int64
	}
	committedEvents := make([]committed, 0, len(dues))
	batchItems := make([]struct {
		eventID, eventType string
		sequence           int64
		payload            string
	}, 0, len(dues))
	for i, d := range dues {
		eventKey, err := core.HashJSON([]string{"rp_activity_settle", suffix, d.id})
		if err != nil {
			return err
		}
		eventID := "event_rp_activity_settle_" + eventKey[7:]
		eventType := "AgentActivityCompleted"
		if d.outcome == "cancelled" {
			eventType = "AgentActivityCancelled"
		}
		payload, err := core.CanonicalJSON(rpActivityEndedEvent{ActivityID: d.id, ActorID: d.actor, PlaceID: d.place, ActivityCode: d.code, StartedWorldTime: d.started, EndedWorldTime: worldTime, Outcome: d.outcome})
		if err != nil {
			return err
		}
		committedEvents = append(committedEvents, committed{d, eventID, sequence + int64(i)})
		batchItems = append(batchItems, struct {
			eventID, eventType string
			sequence           int64
			payload            string
		}{eventID, eventType, sequence + int64(i), string(payload)})
	}
	lastSequence := sequence + int64(len(dues)) - 1
	if err := execAgentOne(ctx, tx.conn, "RP activity settle command", `INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,'RPActivitySettle',?,?,?,?,'{"authorization":"rp-scoped-time-trigger"}','pending',?)`, commandID, instance, branch, "rp_activity_settle:"+suffix, key, head, "system_rp_activity", now); err != nil {
		return err
	}
	if err := execAgentOne(ctx, tx.conn, "RP activity settle attempt", `INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready','corerp-rp-activity',?,?,?)`, commandID, attemptID, s.now().UTC().Add(30*time.Second).Format(time.RFC3339Nano), key, now); err != nil {
		return err
	}
	if err := execAgentOne(ctx, tx.conn, "RP activity settle batch", `INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,?,?,?,?)`, batchID, commandID, instance, branch, epochID, head, sequence, lastSequence, len(dues), worldTime, key, now); err != nil {
		return err
	}
	for i, c := range committedEvents {
		item := batchItems[i]
		if err := execAgentOne(ctx, tx.conn, "RP activity settle event", `INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,?,?,?,?,?)`, item.eventID, batchID, instance, branch, item.sequence, i, item.eventType, c.due.actor, worldTime, item.payload); err != nil {
			return err
		}
	}
	for _, c := range committedEvents {
		if err := execAgentOne(ctx, tx.conn, "RP activity terminal state", `UPDATE rp_activities SET status=?, end_event_id=?, last_event_sequence=? WHERE activity_id=? AND status='in_progress'`, c.due.outcome, c.eventID, c.sequence, c.due.id); err != nil {
			return err
		}
		if err := execAgentOne(ctx, tx.conn, "RP activity own-action record", `INSERT INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence) VALUES (?,?,'activity',?,NULL,?,?,?,?,?,?)`, c.due.actor, c.eventID, c.due.code, c.due.place, worldTime, c.due.outcome, instance, branch, c.sequence); err != nil {
			return err
		}
		if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+c.eventID, "agent_decision", c.eventID, commandID, attemptID, worldTime, now, batchPayloadForAudit(batchItems, c.eventID)); err != nil {
			return err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "RP activity settle clock lineage", `UPDATE world_clocks SET projection_version=projection_version+1, last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, lastSequence, instance, branch, worldTime); err != nil {
		return err
	}
	if err := execAgentOne(ctx, tx.conn, "RP activity settle branch", `UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, lastSequence, instance, branch, head); err != nil {
		return err
	}
	if err := execAgentOne(ctx, tx.conn, "RP activity settle commit attempt", `UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE command_id=? AND attempt_no=1 AND status='ready'`, now, commandID); err != nil {
		return err
	}
	if err := execAgentOne(ctx, tx.conn, "RP activity settle commit command", `UPDATE commands SET status='committed' WHERE command_id=? AND status='pending'`, commandID); err != nil {
		return err
	}
	if finish != nil {
		if err := finish(tx.conn, sequence, lastSequence); err != nil {
			return err
		}
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func batchPayloadForAudit(items []struct {
	eventID, eventType string
	sequence           int64
	payload            string
}, eventID string) []byte {
	for _, item := range items {
		if item.eventID == eventID {
			return []byte(item.payload)
		}
	}
	return []byte("{}")
}

// insertRPOwnAction records an actor's committed effect on the autobiographical
// channel. Migration tests intentionally exercise commit paths against schemas
// older than 058, where the derived projection does not exist yet; migration
// 058 backfills those events. Once 058 is recorded, a missing table is a real
// schema failure and must not be hidden.
func insertRPOwnAction(ctx context.Context, conn *sql.Conn, agentID, eventID, action, activityCode, text, placeID, worldTime, status string, instance, branch string, sequence int64) error {
	available, err := rpActivityContinuityTableAvailable(ctx, conn, "rp_own_actions")
	if err != nil {
		return err
	}
	if !available {
		return nil
	}
	return execAgentOne(ctx, conn, "RP own-action record", `INSERT OR IGNORE INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, agentID, eventID, action, nullable(activityCode), nullable(text), placeID, worldTime, nullable(status), instance, branch, sequence)
}

func rpActivityContinuityTableAvailable(ctx context.Context, q replayQuerier, table string) (bool, error) {
	return rpTableAvailableBeforeMigration(ctx, q, table, RPActivityContinuitySchemaVersion)
}

func rpTableAvailableBeforeMigration(ctx context.Context, q replayQuerier, table, schemaVersion string) (bool, error) {
	var tableCount int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&tableCount); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "inspect RP optional table", err)
	}
	if tableCount != 0 {
		return true, nil
	}
	var versionCount int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, schemaVersion).Scan(&versionCount); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "inspect RP optional table schema version", err)
	}
	if versionCount != 0 {
		return false, core.NewError(core.CodeStorageFailure, "RP optional table "+table+" is missing after migration")
	}
	return false, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
