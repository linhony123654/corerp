package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

type RPJourneyCancelFact struct {
	Version        string `json:"version"`
	JourneyID      string `json:"journey_id"`
	AgentID        string `json:"agent_id"`
	SegmentPlaceID string `json:"segment_place_id"`
	ArrivalItemID  string `json:"arrival_item_id"`
	ScheduleID     string `json:"schedule_id"`
}

type RPJourneyCancelRecord = privateFactRecord[RPJourneyCancelFact]

// Cancellation stops the pending arrival; it does not teleport the actor.
// The segment remains their actual position until a later explicit move.
func (s *Store) CancelRPJourney(ctx context.Context, r core.RPJourneyCancelRequest) (RPJourneyCancelRecord, error) {
	if err := r.Validate(); err != nil {
		return RPJourneyCancelRecord{}, err
	}
	session, err := loadRPSessionRecord(ctx, s.db, r.PrincipalID, r.SessionID)
	if err != nil {
		return RPJourneyCancelRecord{}, err
	}
	binding := core.CareerBinding{PrincipalID: r.PrincipalID, InstanceID: session.InstanceID, BranchID: session.BranchID, ExpectedHead: r.ExpectedCursor, IdempotencyKey: "journey-cancel:" + r.SessionID + ":" + r.IdempotencyKey}
	return executePrivateFactCommandWithOptions(s, ctx, binding, "CancelRPJourney", r,
		privateFactDomain{"rp_journey_cancel", "RPJourneyCancelled", `{"authorization":"rp-session-control"}`},
		privateFactOptions{replayAuthorize: func(conn *sql.Conn) error {
			_, err := loadRPSessionRecord(ctx, conn, r.PrincipalID, r.SessionID)
			return err
		}},
		func(conn *sql.Conn) error {
			current, err := loadRPSession(ctx, conn, r.PrincipalID, r.SessionID)
			if err != nil {
				return err
			}
			if err := authorizeRPControl(ctx, conn, r.PrincipalID, current.InstanceID, current.BranchID, current.ControlledEntityID); err != nil {
				return err
			}
			return requireNoActiveRPSharedRound(ctx, conn, current.InstanceID, current.BranchID)
		},
		func(conn *sql.Conn, c privateFactContext) (RPJourneyCancelFact, func() error, error) {
			current, err := loadRPSession(ctx, conn, r.PrincipalID, r.SessionID)
			if err != nil {
				return RPJourneyCancelFact{}, nil, err
			}
			if current.Status != "active" || current.ObservationCursor != r.ExpectedCursor || current.InstanceID != binding.InstanceID || current.BranchID != binding.BranchID {
				return RPJourneyCancelFact{}, nil, core.NewError(core.CodeBranchConflict, "journey cancellation requires a current active observation")
			}
			var fact RPJourneyCancelFact
			var queueStatus, scheduleStatus string
			err = conn.QueryRowContext(ctx, `SELECT j.agent_id,j.segment_place_id,j.arrival_item_id,j.arrival_schedule_id,q.status,a.status
				FROM rp_journeys j JOIN scheduler_items q ON q.scheduler_item_id=j.arrival_item_id
				JOIN agent_schedule_entries a ON a.schedule_id=j.arrival_schedule_id
				JOIN agent_positions p ON p.agent_id=j.agent_id AND p.place_id=j.segment_place_id
				WHERE j.journey_id=? AND j.instance_id=? AND j.branch_id=? AND j.agent_id=? AND j.status='active'
				AND q.instance_id=j.instance_id AND q.branch_id=j.branch_id AND a.scheduler_item_id=q.scheduler_item_id`,
				r.JourneyID, binding.InstanceID, binding.BranchID, current.ControlledEntityID).Scan(&fact.AgentID, &fact.SegmentPlaceID, &fact.ArrivalItemID, &fact.ScheduleID, &queueStatus, &scheduleStatus)
			if err != nil {
				return RPJourneyCancelFact{}, nil, classifyMissing(err, "active journey to cancel")
			}
			if queueStatus != "pending" || scheduleStatus != "active" {
				return RPJourneyCancelFact{}, nil, core.NewError(core.CodeProjectionDiverged, "journey cancellation queue differs")
			}
			fact.Version, fact.JourneyID = "corerp.spatial.journey-cancel.v1", r.JourneyID
			return fact, func() error {
				if err := execAgentOne(ctx, conn, "cancel journey queue", `UPDATE scheduler_items SET status='cancelled' WHERE scheduler_item_id=? AND status='pending'`, fact.ArrivalItemID); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "cancel journey schedule", `UPDATE agent_schedule_entries SET status='cancelled' WHERE schedule_id=? AND scheduler_item_id=? AND status='active'`, fact.ScheduleID, fact.ArrivalItemID); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "cancel active journey", `UPDATE rp_journeys SET status='cancelled',resolved_event_id=? WHERE journey_id=? AND status='active' AND arrival_item_id=?`, c.EventID, r.JourneyID, fact.ArrivalItemID)
			}, nil
		})
}
