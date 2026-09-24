package storage

import (
	"context"
	"database/sql"
	"time"

	"corerp.local/backend/internal/core"
)

// Selected offers count even if a character later chooses silence. Reused
// receipts retain their original time and identity, not the containing wait's
// time. Shared conditions (weather/roads) are not actor invitations.
func rpOpportunityHistoryPressure(history []rpWaitEvent, actor string, at time.Time, policy RPOpportunityPolicy) (int, bool, error) {
	seen := map[string]bool{}
	cooling := false
	start := at.Add(-time.Duration(policy.HistoryHours) * time.Hour)
	add := func(owner, when string, draw core.RPOpportunityDraw) error {
		if owner != actor || !draw.Selected {
			return nil
		}
		previous, err := time.Parse(time.RFC3339, when)
		if err != nil {
			return err
		}
		if previous.Before(start) || previous.After(at) {
			return nil
		}
		if draw.IdentityHash == "" {
			return core.NewError(core.CodeProjectionDiverged, "selected opportunity lacks identity")
		}
		seen[draw.IdentityHash] = true
		cooling = cooling || at.Before(previous.Add(time.Duration(policy.CooldownHours)*time.Hour))
		return nil
	}
	for _, wait := range history {
		for _, receipt := range wait.VisitOpportunities {
			if err := add(receipt.ActorID, receipt.WorldTime, receipt.Draw); err != nil {
				return 0, false, err
			}
		}
		for _, receipt := range wait.CommunityOpportunities {
			if err := add(receipt.ActorID, receipt.WorldTime, receipt.Draw); err != nil {
				return 0, false, err
			}
		}
		for _, receipt := range wait.ContactOpportunities {
			if err := add(receipt.ActorID, receipt.WorldTime, receipt.Draw); err != nil {
				return 0, false, err
			}
		}
		for _, receipt := range wait.StoreOpportunities {
			if err := add(receipt.ActorID, receipt.WorldTime, receipt.Draw); err != nil {
				return 0, false, err
			}
		}
		for _, receipt := range wait.WorkOpportunities {
			if err := add(receipt.ActorID, receipt.WorldTime, receipt.Draw); err != nil {
				return 0, false, err
			}
		}
	}
	return len(seen), cooling, nil
}

// A new policy or newly materialized actor has no full observation window.
// Ordinary speech/routine is compatible with quiet; actual known work changes
// and selected invitations are not. This does not impose an event floor.
func readRPQuietOpportunityHistory(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput, policy OpportunityPolicyRecord, at time.Time, recent, major int) (bool, error) {
	if input.Life == nil || recent != 0 || major != 0 {
		return false, nil
	}
	start := at.Add(-time.Duration(policy.Fact.Policy.HistoryHours) * time.Hour)
	installed, err := time.Parse(time.RFC3339, policy.WorldTime)
	if err != nil {
		return false, err
	}
	if installed.After(start) {
		return false, nil
	}
	var mature int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND world_time<=?`, input.Life.RoutineSourceEventID, input.InstanceID, input.BranchID, start.Format(time.RFC3339)).Scan(&mature); err != nil {
		return false, err
	}
	if mature != 1 {
		return false, nil
	}
	if input.Law != nil {
		changes, err := readRPCommunityChangeSources(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID, start.Format(time.RFC3339), at.Format(time.RFC3339))
		if err != nil {
			return false, err
		}
		if len(changes) != 0 {
			return false, nil
		}
	}
	for _, memory := range input.Life.SalientMemories {
		if !core.RPWorkChangeMemory(memory, input.NPCEntityID) {
			continue
		}
		when, err := time.Parse(time.RFC3339, memory.WorldTime)
		if err != nil {
			return false, err
		}
		if !when.Before(start) && !when.After(at) {
			return false, nil
		}
	}
	return true, nil
}

// A received personal job-loss notice is a major recent change even before its
// effective end date. It is not proof that employment already ended. Other
// actors' notices and private managerial assessments must not add pressure.
func readRPOpportunityMajorChanges(ctx context.Context, conn *sql.Conn, instance, branch, actor, from, through string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND world_time>=? AND world_time<=? AND json_extract(payload,'$.employment.employee_id')=? AND json_extract(payload,'$.employment_change.kind') IN ('layoff','termination') ORDER BY event_sequence DESC LIMIT 8`, instance, branch, from, through, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}
