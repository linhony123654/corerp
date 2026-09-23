package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// Absence of a proceeding is not authority by itself. A committed scoped
// no-action review permits an explicit zero-money outcome for estate tasks.
func prepareM2EstateWithoutProceeding(ctx context.Context, conn *sql.Conn, item SchedulerItem, p m2EstatePolicy) (*scheduledMutation, error) {
	var source string
	err := conn.QueryRowContext(ctx, `SELECT e.event_id FROM m2_insolvency_reviews r JOIN events e ON e.event_id=r.event_id AND e.event_sequence=r.event_sequence WHERE r.policy_id=? AND r.status='no_action' AND e.instance_id=? AND e.branch_id=? AND e.event_type='M2InsolvencyReviewed' AND e.world_time=? AND json_extract(e.payload,'$.actor_id')=? AND json_extract(e.payload,'$.policy_id')=r.policy_id AND json_extract(e.payload,'$.status')=r.status AND json_extract(e.payload,'$.reason_code')=r.reason_code AND json_extract(e.payload,'$.cash_minor')=r.cash_minor AND json_extract(e.payload,'$.unpaid_wage_minor')=r.unpaid_wage_minor AND json_extract(e.payload,'$.expired_case_count')=r.expired_case_count`, m2InsolvencyPolicyID, M2DemoInstanceID, M2DemoBranchID, m2WageTime(30, 7, 12), p.Actor).Scan(&source)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var proceedings int
	if err := conn.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM m2_bankruptcy_proceedings WHERE actor_id=?)+(SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='M2BankruptcyProceedingOpened' AND json_extract(payload,'$.actor_id')=?)`, p.Actor, M2DemoInstanceID, M2DemoBranchID, p.Actor).Scan(&proceedings); err != nil {
		return nil, err
	}
	if proceedings != 0 {
		return nil, core.NewError(core.CodeProjectionDiverged, "no-action review conflicts with bankruptcy proceeding")
	}
	table, eventType := "m2_estate_contributions", "M2EstateContributionDeferred"
	if item.PhaseID == m2EstateDistributionPhase {
		var prior int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_estate_contributions c JOIN events e ON e.event_id=c.event_id AND e.event_sequence=c.event_sequence WHERE c.policy_id=? AND c.status='deferred' AND c.amount_minor=0 AND c.reason_code='no_bankruptcy_proceeding' AND e.event_type='M2EstateContributionDeferred' AND json_extract(e.payload,'$.review_event_id')=?`, m2EstatePolicyID, source).Scan(&prior); err != nil {
			return nil, err
		}
		if prior != 1 {
			return nil, core.NewError(core.CodeProjectionDiverged, "unopened estate lacks deferred contribution source")
		}
		table, eventType = "m2_estate_distributions", "M2EstateDistributionDeferred"
	}
	return &scheduledMutation{EventType: eventType, EventPayload: struct {
		PolicyID      string `json:"policy_id"`
		ReviewEventID string `json:"review_event_id"`
		Reason        string `json:"reason_code"`
		Amount        int64  `json:"amount_minor"`
	}{m2EstatePolicyID, source, "no_bankruptcy_proceeding", 0}, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		return execAgentOne(ctx, conn, "record estate task without a proceeding", `INSERT INTO `+table+`(scheduler_item_id,policy_id,status,reason_code,amount_minor,event_id,event_sequence) VALUES (?,?,'deferred','no_bankruptcy_proceeding',0,?,?)`, item.SchedulerItemID, m2EstatePolicyID, eventID, sequence)
	}}, nil
}
