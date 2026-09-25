package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type ReplayedAccount struct {
	AccountID    string `json:"account_id"`
	BalanceMinor int64  `json:"balance_minor"`
}

type ReplayedInventory struct {
	LocationID    string `json:"location_id"`
	SKUID         string `json:"sku_id"`
	QuantityMinor int64  `json:"quantity_minor"`
}

type ReplayedPopulation struct {
	OwnerKind       string `json:"owner_kind"`
	OwnerID         string `json:"owner_id"`
	PopulationCount int64  `json:"population_count"`
}

type ReplayedAgentPosition struct {
	AgentID            string `json:"agent_id"`
	PlaceID            string `json:"place_id"`
	ActivityCode       string `json:"activity_code"`
	EffectiveWorldTime string `json:"effective_world_time"`
	SourceEventID      string `json:"source_event_id"`
	LastEventSequence  int64  `json:"last_event_sequence"`
}

type ReplayedAgentKnowledge struct {
	ObserverAgentID   string `json:"observer_agent_id"`
	ClaimKey          string `json:"claim_key"`
	SubjectAgentID    string `json:"subject_agent_id"`
	PlaceID           string `json:"place_id"`
	SourceEventID     string `json:"source_event_id"`
	ObservationID     string `json:"observation_id"`
	LearnedWorldTime  string `json:"learned_world_time"`
	ClaimPayload      string `json:"claim_payload"`
	LastEventSequence int64  `json:"last_event_sequence"`
}

type ReplayState struct {
	InstanceID         string                   `json:"instance_id"`
	BranchID           string                   `json:"branch_id"`
	ThroughSequence    int64                    `json:"through_sequence"`
	AccountBalances    []ReplayedAccount        `json:"account_balances"`
	InventoryBalances  []ReplayedInventory      `json:"inventory_balances"`
	PopulationBalances []ReplayedPopulation     `json:"population_balances,omitempty"`
	AgentPositions     []ReplayedAgentPosition  `json:"agent_positions,omitempty"`
	AgentKnowledge     []ReplayedAgentKnowledge `json:"agent_knowledge,omitempty"`
}

type ReplayResult struct {
	State          ReplayState `json:"state"`
	StateHash      string      `json:"state_hash"`
	UsedSnapshotID string      `json:"used_snapshot_id,omitempty"`
}

type SnapshotInfo struct {
	SnapshotID      string `json:"snapshot_id"`
	ThroughSequence int64  `json:"through_sequence"`
	StateHash       string `json:"state_hash"`
}

type ProjectionDifference struct {
	Projection   string `json:"projection"`
	Key          string `json:"key"`
	Expected     int64  `json:"expected"`
	Actual       int64  `json:"actual"`
	ExpectedText string `json:"expected_text,omitempty"`
	ActualText   string `json:"actual_text,omitempty"`
}

type replayQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) Replay(ctx context.Context, instanceID, branchID string, throughSequence int64) (ReplayResult, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "acquire replay connection", err)
	}
	defer conn.Close()
	return replayRange(ctx, conn, instanceID, branchID, ReplayState{
		InstanceID: instanceID, BranchID: branchID,
	}, 0, throughSequence)
}

func (s *Store) ReplayFromLatestSnapshot(ctx context.Context, instanceID, branchID string, throughSequence int64) (ReplayResult, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "acquire snapshot replay connection", err)
	}
	defer conn.Close()
	if err := validateReplayRange(ctx, conn, instanceID, branchID, throughSequence); err != nil {
		return ReplayResult{}, err
	}

	var snapshotID, stateHash, payload string
	var snapshotThrough int64
	err = conn.QueryRowContext(ctx, `
		SELECT s.snapshot_id, s.through_sequence, s.state_hash, p.payload
		FROM snapshots s JOIN snapshot_payloads p ON p.snapshot_id = s.snapshot_id
		WHERE s.instance_id = ? AND s.branch_id = ? AND s.through_sequence <= ?
		ORDER BY s.through_sequence DESC, s.snapshot_id DESC LIMIT 1`,
		instanceID, branchID, throughSequence,
	).Scan(&snapshotID, &snapshotThrough, &stateHash, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return replayRange(ctx, conn, instanceID, branchID, ReplayState{
			InstanceID: instanceID, BranchID: branchID,
		}, 0, throughSequence)
	}
	if err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "load replay snapshot", err)
	}
	var state ReplayState
	if err := json.Unmarshal([]byte(payload), &state); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeSnapshotMismatch, "decode snapshot payload", err)
	}
	if state.InstanceID != instanceID || state.BranchID != branchID || state.ThroughSequence != snapshotThrough {
		return ReplayResult{}, core.NewError(core.CodeSnapshotMismatch, "snapshot identity or high-water mark does not match metadata")
	}
	computedHash, err := core.HashJSON(state)
	if err != nil {
		return ReplayResult{}, err
	}
	if computedHash != stateHash {
		return ReplayResult{}, core.NewError(core.CodeSnapshotMismatch, "snapshot payload hash does not match metadata")
	}
	result, err := replayRange(ctx, conn, instanceID, branchID, state, snapshotThrough, throughSequence)
	if err != nil {
		return ReplayResult{}, err
	}
	result.UsedSnapshotID = snapshotID
	return result, nil
}

func (s *Store) CreateSnapshot(ctx context.Context, instanceID, branchID string, throughSequence int64) (SnapshotInfo, error) {
	replay, err := s.Replay(ctx, instanceID, branchID, throughSequence)
	if err != nil {
		return SnapshotInfo{}, err
	}
	payload, err := core.CanonicalJSON(replay.State)
	if err != nil {
		return SnapshotInfo{}, err
	}
	snapshotID := fmt.Sprintf("snapshot_%s_%s_%d_%s", instanceID, branchID, throughSequence, replay.StateHash[7:19])
	storageRef := "sqlite:snapshot_payloads/" + snapshotID
	now := s.now().UTC().Format(time.RFC3339Nano)

	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return SnapshotInfo{}, core.WrapError(core.CodeStorageFailure, "begin snapshot write", err)
	}
	defer tx.Rollback(ctx)
	result, err := tx.conn.ExecContext(ctx, `
		INSERT INTO snapshots(snapshot_id, instance_id, branch_id, through_sequence, state_hash, storage_ref, created_at_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(instance_id, branch_id, through_sequence) DO NOTHING`,
		snapshotID, instanceID, branchID, throughSequence, replay.StateHash, storageRef, now,
	)
	if err != nil {
		return SnapshotInfo{}, core.WrapError(core.CodeStorageFailure, "insert snapshot metadata", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return SnapshotInfo{}, core.WrapError(core.CodeStorageFailure, "inspect snapshot insert", err)
	}
	if rows == 1 {
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO snapshot_payloads(snapshot_id, payload) VALUES (?, ?)`, snapshotID, string(payload)); err != nil {
			return SnapshotInfo{}, core.WrapError(core.CodeStorageFailure, "insert snapshot payload", err)
		}
	} else {
		var existingID, existingHash string
		if err := tx.conn.QueryRowContext(ctx, `SELECT snapshot_id, state_hash FROM snapshots WHERE instance_id = ? AND branch_id = ? AND through_sequence = ?`, instanceID, branchID, throughSequence).Scan(&existingID, &existingHash); err != nil {
			return SnapshotInfo{}, core.WrapError(core.CodeStorageFailure, "load existing snapshot", err)
		}
		if existingHash != replay.StateHash {
			return SnapshotInfo{}, core.NewError(core.CodeSnapshotMismatch, "existing snapshot hash differs from authoritative replay")
		}
		snapshotID = existingID
	}
	if err := tx.Commit(ctx); err != nil {
		return SnapshotInfo{}, core.WrapError(core.CodeStorageFailure, "commit snapshot", err)
	}
	return SnapshotInfo{SnapshotID: snapshotID, ThroughSequence: throughSequence, StateHash: replay.StateHash}, nil
}

func (s *Store) CompareProjections(ctx context.Context, instanceID, branchID string) ([]ProjectionDifference, error) {
	var head int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, instanceID, branchID).Scan(&head); err != nil {
		return nil, classifyMissing(err, "branch")
	}
	replay, err := s.Replay(ctx, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	differences, err := compareProjectionRows(ctx, s.db, replay.State)
	if err != nil {
		return nil, err
	}
	studio, err := studioAccessDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	differences = append(differences, studio...)
	activation, err := studioActivationDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	differences = append(differences, activation...)
	readiness, err := studioReadinessDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	differences = append(differences, readiness...)
	wages, err := careerWageProjectionDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	roles, err := careerRoleProjectionDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	culture, err := cultureGrantProjectionDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	institutions, err := institutionGrantProjectionDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	transit, err := transitProjectionDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	leaveQueues, err := careerLeaveQueueDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	locations, err := rpLocationProjectionDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	journeys, err := rpJourneyProjectionDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	journeyQueues, err := rpJourneyQueueDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	perception, err := rpPerceptionProjectionDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	identity, err := rpIdentityProjectionDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	enrollments, err := rpExternalEnrollmentDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	authorities, err := rpControllerAuthorityDifferences(ctx, s.db, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	for _, group := range [][]ProjectionDifference{wages, roles, culture, institutions, transit, leaveQueues, locations, journeys, journeyQueues, perception, identity, enrollments, authorities} {
		differences = append(differences, group...)
	}
	return differences, nil
}

func (s *Store) RebuildProjections(ctx context.Context, instanceID, branchID string) error {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "begin projection rebuild", err)
	}
	defer tx.Rollback(ctx)
	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, instanceID, branchID).Scan(&head); err != nil {
		return classifyMissing(err, "branch")
	}
	replay, err := replayRange(ctx, tx.conn, instanceID, branchID, ReplayState{
		InstanceID: instanceID, BranchID: branchID,
	}, 0, head)
	if err != nil {
		return err
	}
	locations, err := rpLocationProjectionDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairRPLocationProjections(ctx, tx.conn, instanceID, branchID, locations); err != nil {
		return err
	}
	journeys, err := rpJourneyProjectionDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairRPJourneyProjections(ctx, tx.conn, instanceID, branchID, journeys); err != nil {
		return err
	}
	journeyQueues, err := rpJourneyQueueDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairRPJourneyQueues(ctx, tx.conn, instanceID, branchID, journeyQueues); err != nil {
		return err
	}
	perception, err := rpPerceptionProjectionDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairRPPerceptionProjections(ctx, tx.conn, instanceID, branchID, perception); err != nil {
		return err
	}
	identity, err := rpIdentityProjectionDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairRPIdentityProjections(ctx, tx.conn, instanceID, branchID, identity); err != nil {
		return err
	}
	enrollments, err := rpExternalEnrollmentDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if len(enrollments) != 0 {
		// Authority rows reference enrollment keys. Drop derived authority
		// inside this rebuild transaction before replacing damaged enrollment;
		// the sourced authority projection is restored immediately below.
		if _, err := tx.conn.ExecContext(ctx, `DELETE FROM rp_controller_authorities WHERE instance_id=? AND branch_id=?`, instanceID, branchID); err != nil {
			return err
		}
	}
	if err := repairRPExternalEnrollments(ctx, tx.conn, instanceID, branchID, head, enrollments); err != nil {
		return err
	}
	authorities, err := rpControllerAuthorityDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairRPControllerAuthorities(ctx, tx.conn, instanceID, branchID, head, authorities); err != nil {
		return err
	}
	for _, account := range replay.State.AccountBalances {
		result, err := tx.conn.ExecContext(ctx, `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ?`, account.BalanceMinor, head, account.AccountID)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "rebuild account projection", err)
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			return core.NewError(core.CodeProjectionDiverged, "authoritative account has no projection row: "+account.AccountID)
		}
	}
	for _, inventory := range replay.State.InventoryBalances {
		if _, err := tx.conn.ExecContext(ctx, `
			INSERT INTO inventory_balances(location_id, sku_id, quantity_minor, projection_version, last_event_sequence)
			VALUES (?, ?, ?, 1, ?)
			ON CONFLICT(location_id, sku_id) DO UPDATE SET
			  quantity_minor = excluded.quantity_minor,
			  projection_version = inventory_balances.projection_version + 1,
			  last_event_sequence = excluded.last_event_sequence`,
			inventory.LocationID, inventory.SKUID, inventory.QuantityMinor, head,
		); err != nil {
			return core.WrapError(core.CodeStorageFailure, "rebuild inventory projection", err)
		}
	}
	for _, population := range replay.State.PopulationBalances {
		var result sql.Result
		var err error
		status := replayedPopulationStatus(population.OwnerKind, population.PopulationCount)
		switch population.OwnerKind {
		case "cohort":
			result, err = tx.conn.ExecContext(ctx, `UPDATE cohorts SET population_count = ?, status = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE cohort_id = ? AND instance_id = ? AND branch_id = ?`, population.PopulationCount, status, head, population.OwnerID, instanceID, branchID)
		case "entity":
			result, err = tx.conn.ExecContext(ctx, `
				UPDATE materialized_entities SET population_count = ?, status = ?, projection_version = projection_version + 1, last_event_sequence = ?
				WHERE entity_id = ? AND source_cohort_id IN (SELECT cohort_id FROM cohorts WHERE instance_id = ? AND branch_id = ?)`,
				population.PopulationCount, status, head, population.OwnerID, instanceID, branchID)
		default:
			return core.NewError(core.CodeProjectionDiverged, "unknown replay population owner kind: "+population.OwnerKind)
		}
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "rebuild population projection", err)
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			return core.NewError(core.CodeProjectionDiverged, "authoritative population has no projection row: "+populationKey(population.OwnerKind, population.OwnerID))
		}
	}
	for _, position := range replay.State.AgentPositions {
		result, err := tx.conn.ExecContext(ctx, `
			UPDATE agent_positions SET place_id = ?, activity_code = ?, effective_world_time = ?,
			  projection_version = projection_version + 1, last_event_sequence = ?
			WHERE agent_id = ? AND agent_id IN (
			  SELECT agent_id FROM agent_profiles WHERE instance_id = ? AND branch_id = ?
			)`, position.PlaceID, position.ActivityCode, position.EffectiveWorldTime,
			position.LastEventSequence, position.AgentID, instanceID, branchID)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "rebuild Agent position projection", err)
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			return core.NewError(core.CodeProjectionDiverged, "authoritative Agent position has no projection row: "+position.AgentID)
		}
	}
	if _, err := tx.conn.ExecContext(ctx, `DELETE FROM agent_knowledge WHERE observer_agent_id IN (SELECT agent_id FROM agent_profiles WHERE instance_id = ? AND branch_id = ?)`, instanceID, branchID); err != nil {
		return core.WrapError(core.CodeStorageFailure, "clear Agent knowledge projections", err)
	}
	for _, knowledge := range replay.State.AgentKnowledge {
		if _, err := tx.conn.ExecContext(ctx, `
			INSERT INTO agent_knowledge(observer_agent_id, claim_key, subject_agent_id, place_id, source_event_id, observation_id, learned_world_time, claim_payload, projection_version, last_event_sequence)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`, knowledge.ObserverAgentID, knowledge.ClaimKey,
			knowledge.SubjectAgentID, knowledge.PlaceID, knowledge.SourceEventID, knowledge.ObservationID,
			knowledge.LearnedWorldTime, knowledge.ClaimPayload, knowledge.LastEventSequence); err != nil {
			return core.WrapError(core.CodeStorageFailure, "rebuild Agent knowledge projection", err)
		}
	}
	wages, err := careerWageProjectionDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	for _, wage := range wages {
		if wage.Projection == "career_contract_status" {
			if err := execAgentOne(ctx, tx.conn, "rebuild career contract status", `UPDATE employment_contracts SET status=? WHERE contract_id=? AND status=?`, wage.ExpectedText, wage.Key, wage.ActualText); err != nil {
				return err
			}
			continue
		}
		if wage.Projection == "career_contract_position" {
			if err := execAgentOne(ctx, tx.conn, "rebuild career position projection", `UPDATE employment_contracts SET position_id=? WHERE contract_id=? AND position_id=?`, wage.ExpectedText, wage.Key, wage.ActualText); err != nil {
				return err
			}
			continue
		}
		if err := execAgentOne(ctx, tx.conn, "rebuild career wage projection", `UPDATE employment_contracts SET gross_wage_minor=? WHERE contract_id=? AND gross_wage_minor=?`, wage.Expected, wage.Key, wage.Actual); err != nil {
			return err
		}
	}
	studio, studioErr := studioAccessDifferences(ctx, tx.conn, instanceID, branchID, head)
	if studioErr != nil {
		return studioErr
	}
	if err := repairStudioAccess(ctx, tx.conn, instanceID, branchID, studio); err != nil {
		return err
	}
	if err := repairStudioActivation(ctx, tx.conn, instanceID, branchID, head); err != nil {
		return err
	}
	if err := repairStudioReadiness(ctx, tx.conn, instanceID, branchID, head); err != nil {
		return err
	}
	roles, err := careerRoleProjectionDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairCareerRoleProjections(ctx, tx.conn, instanceID, branchID, roles); err != nil {
		return err
	}
	culture, err := cultureGrantProjectionDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairCultureGrantProjections(ctx, tx.conn, culture); err != nil {
		return err
	}
	institutions, err := institutionGrantProjectionDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairInstitutionGrantProjections(ctx, tx.conn, institutions); err != nil {
		return err
	}
	transit, err := transitProjectionDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairTransitProjections(ctx, tx.conn, instanceID, branchID, transit); err != nil {
		return err
	}
	leaveQueues, err := careerLeaveQueueDifferences(ctx, tx.conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if err := repairCareerLeaveQueues(ctx, tx.conn, instanceID, branchID, leaveQueues); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit projection rebuild", err)
	}
	return nil
}

func replayRange(ctx context.Context, query replayQuerier, instanceID, branchID string, base ReplayState, afterSequence, throughSequence int64) (ReplayResult, error) {
	if err := validateReplayRange(ctx, query, instanceID, branchID, throughSequence); err != nil {
		return ReplayResult{}, err
	}
	accounts, inventory, populations := replayMaps(base)
	positions, knowledge := replayAgentMaps(base)
	rows, err := query.QueryContext(ctx, `
		SELECT e.event_sequence, p.account_id, p.amount_minor
		FROM postings p
		JOIN journal_entries j ON j.entry_id = p.entry_id AND j.status = 'posted'
		JOIN events e ON e.event_id = j.event_id
		WHERE e.instance_id = ? AND e.branch_id = ? AND e.event_sequence > ? AND e.event_sequence <= ?
		ORDER BY e.event_sequence, p.posting_id`, instanceID, branchID, afterSequence, throughSequence)
	if err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "read replay postings", err)
	}
	for rows.Next() {
		var sequence, amount int64
		var accountID string
		if err := rows.Scan(&sequence, &accountID, &amount); err != nil {
			rows.Close()
			return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "scan replay posting", err)
		}
		balance, ok := checkedAdd(accounts[accountID], amount)
		if !ok {
			rows.Close()
			return ReplayResult{}, core.NewError(core.CodeIntegerOverflow, fmt.Sprintf("account %s overflows at sequence %d", accountID, sequence))
		}
		accounts[accountID] = balance
	}
	if err := rows.Close(); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "close replay postings", err)
	}
	if err := rows.Err(); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "iterate replay postings", err)
	}

	rows, err = query.QueryContext(ctx, `
		SELECT e.event_sequence, m.sku_id, m.from_location_id, m.to_location_id, m.quantity_minor, m.movement_kind
		FROM stock_movements m JOIN events e ON e.event_id = m.event_id
		WHERE e.instance_id = ? AND e.branch_id = ? AND e.event_sequence > ? AND e.event_sequence <= ?
		ORDER BY e.event_sequence, m.movement_id`, instanceID, branchID, afterSequence, throughSequence)
	if err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "read replay stock movements", err)
	}
	for rows.Next() {
		var sequence, quantity int64
		var skuID, fromLocation, toLocation, kind string
		if err := rows.Scan(&sequence, &skuID, &fromLocation, &toLocation, &quantity, &kind); err != nil {
			rows.Close()
			return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "scan replay stock movement", err)
		}
		if kind == "transfer" || kind == "consume" || kind == "destroy" {
			key := inventoryKey(fromLocation, skuID)
			value, ok := checkedSubtract(inventory[key], quantity)
			if !ok || value < 0 {
				rows.Close()
				return ReplayResult{}, core.NewError(core.CodeProjectionDiverged, fmt.Sprintf("negative authoritative inventory %s at sequence %d", key, sequence))
			}
			inventory[key] = value
		}
		if kind == "transfer" || kind == "create" {
			key := inventoryKey(toLocation, skuID)
			value, ok := checkedAdd(inventory[key], quantity)
			if !ok {
				rows.Close()
				return ReplayResult{}, core.NewError(core.CodeIntegerOverflow, fmt.Sprintf("inventory %s overflows at sequence %d", key, sequence))
			}
			inventory[key] = value
		}
	}
	if err := rows.Close(); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "close replay stock movements", err)
	}
	if err := rows.Err(); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "iterate replay stock movements", err)
	}

	rows, err = query.QueryContext(ctx, `
		SELECT e.event_sequence, m.from_owner_kind, m.from_owner_id, m.to_owner_kind, m.to_owner_id, m.population_count
		FROM population_movements m JOIN events e ON e.event_id = m.event_id
		WHERE e.instance_id = ? AND e.branch_id = ? AND e.event_sequence > ? AND e.event_sequence <= ?
		ORDER BY e.event_sequence, m.movement_id`, instanceID, branchID, afterSequence, throughSequence)
	if err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "read replay population movements", err)
	}
	for rows.Next() {
		var sequence, count int64
		var fromKind, fromID, toKind, toID sql.NullString
		if err := rows.Scan(&sequence, &fromKind, &fromID, &toKind, &toID, &count); err != nil {
			rows.Close()
			return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "scan replay population movement", err)
		}
		if fromID.Valid {
			key := populationKey(fromKind.String, fromID.String)
			value, ok := checkedSubtract(populations[key], count)
			if !ok || value < 0 {
				rows.Close()
				return ReplayResult{}, core.NewError(core.CodeProjectionDiverged, fmt.Sprintf("negative authoritative population %s at sequence %d", key, sequence))
			}
			populations[key] = value
		}
		if toID.Valid {
			key := populationKey(toKind.String, toID.String)
			value, ok := checkedAdd(populations[key], count)
			if !ok {
				rows.Close()
				return ReplayResult{}, core.NewError(core.CodeIntegerOverflow, fmt.Sprintf("population %s overflows at sequence %d", key, sequence))
			}
			populations[key] = value
		}
	}
	if err := rows.Close(); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "close replay population movements", err)
	}
	if err := rows.Err(); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "iterate replay population movements", err)
	}

	rows, err = query.QueryContext(ctx, `
		SELECT e.event_sequence, e.event_id, m.agent_id, m.to_place_id, m.activity_code, m.world_time
		FROM agent_movements m JOIN events e ON e.event_id = m.event_id
		WHERE e.instance_id = ? AND e.branch_id = ? AND e.event_sequence > ? AND e.event_sequence <= ?
		UNION ALL
		SELECT e.event_sequence,e.event_id,e.actor_id,json_extract(e.payload,'$.to_place_id'),json_extract(e.payload,'$.activity_code'),e.world_time
		FROM events e WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence>? AND e.event_sequence<=? AND e.event_type='AgentActivityStarted'
		ORDER BY 1,3`, instanceID, branchID, afterSequence, throughSequence, instanceID, branchID, afterSequence, throughSequence)
	if err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "read replay Agent movements", err)
	}
	for rows.Next() {
		var position ReplayedAgentPosition
		if err := rows.Scan(&position.LastEventSequence, &position.SourceEventID, &position.AgentID, &position.PlaceID, &position.ActivityCode, &position.EffectiveWorldTime); err != nil {
			rows.Close()
			return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "scan replay Agent movement", err)
		}
		positions[position.AgentID] = position
	}
	if err := rows.Close(); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "close replay Agent movements", err)
	}
	if err := rows.Err(); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "iterate replay Agent movements", err)
	}

	rows, err = query.QueryContext(ctx, `
		SELECT e.event_sequence, o.observer_agent_id, o.claim_key, o.subject_agent_id, o.place_id,
		       o.source_event_id, o.observation_id, o.observed_world_time, o.claim_payload
		FROM observation_records o JOIN events e ON e.event_id = o.source_event_id
		WHERE e.instance_id = ? AND e.branch_id = ? AND e.event_sequence > ? AND e.event_sequence <= ?
		ORDER BY e.event_sequence, o.observation_id`, instanceID, branchID, afterSequence, throughSequence)
	if err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "read replay Agent observations", err)
	}
	for rows.Next() {
		var fact ReplayedAgentKnowledge
		if err := rows.Scan(&fact.LastEventSequence, &fact.ObserverAgentID, &fact.ClaimKey, &fact.SubjectAgentID,
			&fact.PlaceID, &fact.SourceEventID, &fact.ObservationID, &fact.LearnedWorldTime, &fact.ClaimPayload); err != nil {
			rows.Close()
			return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "scan replay Agent observation", err)
		}
		knowledge[agentKnowledgeKey(fact.ObserverAgentID, fact.ClaimKey)] = fact
	}
	if err := rows.Close(); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "close replay Agent observations", err)
	}
	if err := rows.Err(); err != nil {
		return ReplayResult{}, core.WrapError(core.CodeStorageFailure, "iterate replay Agent observations", err)
	}

	state := replayStateFromMaps(instanceID, branchID, throughSequence, accounts, inventory, populations, positions, knowledge)
	hash, err := core.HashJSON(state)
	if err != nil {
		return ReplayResult{}, err
	}
	return ReplayResult{State: state, StateHash: hash}, nil
}

func validateReplayRange(ctx context.Context, query replayQuerier, instanceID, branchID string, throughSequence int64) error {
	if throughSequence < 0 {
		return core.NewError(core.CodeInvalidArgument, "replay sequence must be nonnegative")
	}
	var head int64
	if err := query.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, instanceID, branchID).Scan(&head); err != nil {
		return classifyMissing(err, "branch")
	}
	if throughSequence > head {
		return core.NewError(core.CodeInvalidArgument, fmt.Sprintf("replay sequence %d exceeds branch head %d", throughSequence, head))
	}
	return nil
}

func replayMaps(state ReplayState) (map[string]int64, map[string]int64, map[string]int64) {
	accounts := make(map[string]int64, len(state.AccountBalances))
	for _, account := range state.AccountBalances {
		accounts[account.AccountID] = account.BalanceMinor
	}
	inventory := make(map[string]int64, len(state.InventoryBalances))
	for _, item := range state.InventoryBalances {
		inventory[inventoryKey(item.LocationID, item.SKUID)] = item.QuantityMinor
	}
	populations := make(map[string]int64, len(state.PopulationBalances))
	for _, population := range state.PopulationBalances {
		populations[populationKey(population.OwnerKind, population.OwnerID)] = population.PopulationCount
	}
	return accounts, inventory, populations
}

func replayAgentMaps(state ReplayState) (map[string]ReplayedAgentPosition, map[string]ReplayedAgentKnowledge) {
	positions := make(map[string]ReplayedAgentPosition, len(state.AgentPositions))
	for _, position := range state.AgentPositions {
		positions[position.AgentID] = position
	}
	knowledge := make(map[string]ReplayedAgentKnowledge, len(state.AgentKnowledge))
	for _, fact := range state.AgentKnowledge {
		knowledge[agentKnowledgeKey(fact.ObserverAgentID, fact.ClaimKey)] = fact
	}
	return positions, knowledge
}

func replayStateFromMaps(instanceID, branchID string, throughSequence int64, accounts, inventory, populations map[string]int64, positions map[string]ReplayedAgentPosition, knowledge map[string]ReplayedAgentKnowledge) ReplayState {
	state := ReplayState{InstanceID: instanceID, BranchID: branchID, ThroughSequence: throughSequence}
	for accountID, balance := range accounts {
		state.AccountBalances = append(state.AccountBalances, ReplayedAccount{AccountID: accountID, BalanceMinor: balance})
	}
	sort.Slice(state.AccountBalances, func(left, right int) bool {
		return state.AccountBalances[left].AccountID < state.AccountBalances[right].AccountID
	})
	for key, quantity := range inventory {
		locationID, skuID, _ := strings.Cut(key, "\x1f")
		state.InventoryBalances = append(state.InventoryBalances, ReplayedInventory{LocationID: locationID, SKUID: skuID, QuantityMinor: quantity})
	}
	sort.Slice(state.InventoryBalances, func(left, right int) bool {
		if state.InventoryBalances[left].LocationID != state.InventoryBalances[right].LocationID {
			return state.InventoryBalances[left].LocationID < state.InventoryBalances[right].LocationID
		}
		return state.InventoryBalances[left].SKUID < state.InventoryBalances[right].SKUID
	})
	for key, count := range populations {
		ownerKind, ownerID, _ := strings.Cut(key, "\x1f")
		state.PopulationBalances = append(state.PopulationBalances, ReplayedPopulation{OwnerKind: ownerKind, OwnerID: ownerID, PopulationCount: count})
	}
	sort.Slice(state.PopulationBalances, func(left, right int) bool {
		if state.PopulationBalances[left].OwnerKind != state.PopulationBalances[right].OwnerKind {
			return state.PopulationBalances[left].OwnerKind < state.PopulationBalances[right].OwnerKind
		}
		return state.PopulationBalances[left].OwnerID < state.PopulationBalances[right].OwnerID
	})
	for _, position := range positions {
		state.AgentPositions = append(state.AgentPositions, position)
	}
	sort.Slice(state.AgentPositions, func(left, right int) bool {
		return state.AgentPositions[left].AgentID < state.AgentPositions[right].AgentID
	})
	for _, fact := range knowledge {
		state.AgentKnowledge = append(state.AgentKnowledge, fact)
	}
	sort.Slice(state.AgentKnowledge, func(left, right int) bool {
		if state.AgentKnowledge[left].ObserverAgentID != state.AgentKnowledge[right].ObserverAgentID {
			return state.AgentKnowledge[left].ObserverAgentID < state.AgentKnowledge[right].ObserverAgentID
		}
		return state.AgentKnowledge[left].ClaimKey < state.AgentKnowledge[right].ClaimKey
	})
	return state
}

func compareProjectionRows(ctx context.Context, query replayQuerier, state ReplayState) ([]ProjectionDifference, error) {
	expectedAccounts, expectedInventory, expectedPopulations := replayMaps(state)
	expectedPositions, expectedKnowledge := replayAgentMaps(state)
	differences := make([]ProjectionDifference, 0)
	rows, err := query.QueryContext(ctx, `SELECT account_id, balance_minor FROM account_balances ORDER BY account_id`)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read account projections", err)
	}
	seenAccounts := map[string]bool{}
	for rows.Next() {
		var accountID string
		var actual int64
		if err := rows.Scan(&accountID, &actual); err != nil {
			rows.Close()
			return nil, core.WrapError(core.CodeStorageFailure, "scan account projection", err)
		}
		if expected, relevant := expectedAccounts[accountID]; relevant {
			seenAccounts[accountID] = true
			if actual != expected {
				differences = append(differences, ProjectionDifference{Projection: "account_balance", Key: accountID, Expected: expected, Actual: actual})
			}
		}
	}
	if err := rows.Close(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "close account projections", err)
	}
	for accountID, expected := range expectedAccounts {
		if !seenAccounts[accountID] {
			differences = append(differences, ProjectionDifference{Projection: "account_balance", Key: accountID, Expected: expected})
		}
	}

	rows, err = query.QueryContext(ctx, `SELECT location_id, sku_id, quantity_minor FROM inventory_balances ORDER BY location_id, sku_id`)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read inventory projections", err)
	}
	seenInventory := map[string]bool{}
	for rows.Next() {
		var locationID, skuID string
		var actual int64
		if err := rows.Scan(&locationID, &skuID, &actual); err != nil {
			rows.Close()
			return nil, core.WrapError(core.CodeStorageFailure, "scan inventory projection", err)
		}
		key := inventoryKey(locationID, skuID)
		if expected, relevant := expectedInventory[key]; relevant {
			seenInventory[key] = true
			if actual != expected {
				differences = append(differences, ProjectionDifference{Projection: "inventory_balance", Key: key, Expected: expected, Actual: actual})
			}
		}
	}
	if err := rows.Close(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "close inventory projections", err)
	}
	for key, expected := range expectedInventory {
		if !seenInventory[key] {
			differences = append(differences, ProjectionDifference{Projection: "inventory_balance", Key: key, Expected: expected})
		}
	}
	rows, err = query.QueryContext(ctx, `
		SELECT 'cohort', cohort_id, population_count, status FROM cohorts WHERE instance_id = ? AND branch_id = ?
		UNION ALL
		SELECT 'entity', e.entity_id, e.population_count, e.status FROM materialized_entities e
		JOIN cohorts c ON c.cohort_id = e.source_cohort_id WHERE c.instance_id = ? AND c.branch_id = ?
		ORDER BY 1, 2`, state.InstanceID, state.BranchID, state.InstanceID, state.BranchID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read population projections", err)
	}
	seenPopulations := map[string]bool{}
	for rows.Next() {
		var ownerKind, ownerID, actualStatus string
		var actual int64
		if err := rows.Scan(&ownerKind, &ownerID, &actual, &actualStatus); err != nil {
			rows.Close()
			return nil, core.WrapError(core.CodeStorageFailure, "scan population projection", err)
		}
		key := populationKey(ownerKind, ownerID)
		if expected, relevant := expectedPopulations[key]; relevant {
			seenPopulations[key] = true
			if actual != expected {
				differences = append(differences, ProjectionDifference{Projection: "population_balance", Key: key, Expected: expected, Actual: actual})
			}
			expectedStatus := replayedPopulationStatus(ownerKind, expected)
			if actualStatus != expectedStatus {
				differences = append(differences, ProjectionDifference{Projection: "population_status", Key: key, ExpectedText: expectedStatus, ActualText: actualStatus})
			}
		}
	}
	if err := rows.Close(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "close population projections", err)
	}
	for key, expected := range expectedPopulations {
		if !seenPopulations[key] {
			differences = append(differences, ProjectionDifference{Projection: "population_balance", Key: key, Expected: expected})
		}
	}
	rows, err = query.QueryContext(ctx, `
		SELECT p.agent_id, p.place_id, p.activity_code, p.effective_world_time, e.event_id, p.last_event_sequence
		FROM agent_positions p JOIN agent_profiles a ON a.agent_id = p.agent_id
		JOIN events e ON e.event_sequence=p.last_event_sequence AND e.instance_id=a.instance_id AND e.branch_id=a.branch_id
		LEFT JOIN agent_movements m ON m.agent_id=p.agent_id AND m.event_id=e.event_id
		WHERE a.instance_id = ? AND a.branch_id = ? AND (m.movement_id IS NOT NULL OR (e.event_type='AgentActivityStarted' AND e.actor_id=p.agent_id)) ORDER BY p.agent_id`, state.InstanceID, state.BranchID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read Agent position projections", err)
	}
	seenPositions := map[string]bool{}
	for rows.Next() {
		var actual ReplayedAgentPosition
		if err := rows.Scan(&actual.AgentID, &actual.PlaceID, &actual.ActivityCode, &actual.EffectiveWorldTime, &actual.SourceEventID, &actual.LastEventSequence); err != nil {
			rows.Close()
			return nil, core.WrapError(core.CodeStorageFailure, "scan Agent position projection", err)
		}
		seenPositions[actual.AgentID] = true
		expected, relevant := expectedPositions[actual.AgentID]
		if !relevant || actual != expected {
			differences = append(differences, ProjectionDifference{Projection: "agent_position", Key: actual.AgentID, ExpectedText: formatAgentPosition(expected), ActualText: formatAgentPosition(actual)})
		}
	}
	if err := rows.Close(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "close Agent position projections", err)
	}
	for key, expected := range expectedPositions {
		if !seenPositions[key] {
			differences = append(differences, ProjectionDifference{Projection: "agent_position", Key: key, ExpectedText: formatAgentPosition(expected)})
		}
	}

	rows, err = query.QueryContext(ctx, `
		SELECT k.observer_agent_id, k.claim_key, k.subject_agent_id, k.place_id, k.source_event_id,
		       k.observation_id, k.learned_world_time, k.claim_payload, k.last_event_sequence
		FROM agent_knowledge k JOIN agent_profiles a ON a.agent_id = k.observer_agent_id
		WHERE a.instance_id = ? AND a.branch_id = ? ORDER BY k.observer_agent_id, k.claim_key`, state.InstanceID, state.BranchID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read Agent knowledge projections", err)
	}
	seenKnowledge := map[string]bool{}
	for rows.Next() {
		var actual ReplayedAgentKnowledge
		if err := rows.Scan(&actual.ObserverAgentID, &actual.ClaimKey, &actual.SubjectAgentID, &actual.PlaceID,
			&actual.SourceEventID, &actual.ObservationID, &actual.LearnedWorldTime, &actual.ClaimPayload,
			&actual.LastEventSequence); err != nil {
			rows.Close()
			return nil, core.WrapError(core.CodeStorageFailure, "scan Agent knowledge projection", err)
		}
		key := agentKnowledgeKey(actual.ObserverAgentID, actual.ClaimKey)
		seenKnowledge[key] = true
		expected, relevant := expectedKnowledge[key]
		if !relevant || actual != expected {
			differences = append(differences, ProjectionDifference{Projection: "agent_knowledge", Key: key, ExpectedText: formatAgentKnowledge(expected), ActualText: formatAgentKnowledge(actual)})
		}
	}
	if err := rows.Close(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "close Agent knowledge projections", err)
	}
	for key, expected := range expectedKnowledge {
		if !seenKnowledge[key] {
			differences = append(differences, ProjectionDifference{Projection: "agent_knowledge", Key: key, ExpectedText: formatAgentKnowledge(expected)})
		}
	}
	sort.Slice(differences, func(left, right int) bool {
		if differences[left].Projection != differences[right].Projection {
			return differences[left].Projection < differences[right].Projection
		}
		return differences[left].Key < differences[right].Key
	})
	return differences, nil
}

func inventoryKey(locationID, skuID string) string { return locationID + "\x1f" + skuID }

func populationKey(ownerKind, ownerID string) string { return ownerKind + "\x1f" + ownerID }

func agentKnowledgeKey(observerAgentID, claimKey string) string {
	return observerAgentID + "\x1f" + claimKey
}

func formatAgentPosition(position ReplayedAgentPosition) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d", position.PlaceID, position.ActivityCode, position.EffectiveWorldTime, position.SourceEventID, position.LastEventSequence)
}

func formatAgentKnowledge(knowledge ReplayedAgentKnowledge) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%d", knowledge.SubjectAgentID, knowledge.PlaceID,
		knowledge.SourceEventID, knowledge.ObservationID, knowledge.LearnedWorldTime, knowledge.ClaimKey,
		knowledge.ClaimPayload, knowledge.LastEventSequence)
}

func replayedPopulationStatus(ownerKind string, population int64) string {
	if ownerKind == "cohort" {
		if population == 0 {
			return "depleted"
		}
		return "active"
	}
	if population == 0 {
		return "dematerialized"
	}
	return "active"
}
