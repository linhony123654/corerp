package storage

import (
	"context"
	"database/sql"
	"errors"

	"corerp.local/backend/internal/core"
)

// Publication retains its source Agent principal as the Event actor. An F3
// controller is an additional, exact selected-child authority, never a
// substitute for the original Career/Institution grant.
func requireRPSelectedNoticePublishWindow(ctx context.Context, conn *sql.Conn, actionKind string,
	b core.CareerBinding, controllerPrincipal, sessionID, speakerID string, request any) error {
	if sessionID == "" || controllerPrincipal == "" {
		if sessionID != controllerPrincipal {
			return core.NewError(core.CodeInvalidArgument, "notice controller pair required")
		}
		if err := requireNoActiveRPSharedRound(ctx, conn, b.InstanceID, b.BranchID); err != nil {
			return err
		}
		var external int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? AND status='active'`,
			b.InstanceID, b.BranchID).Scan(&external); err != nil {
			return err
		}
		if external != 0 {
			return core.NewError(core.CodeCommandInProgress, "notice publication requires a shared action window with external residents")
		}
		return nil
	}
	var roundID, raw string
	var baseline int64
	err := conn.QueryRowContext(ctx, `SELECT r.round_id,r.baseline_head,a.request_json FROM rp_shared_rounds r
	 JOIN rp_shared_round_actions a ON a.round_id=r.round_id AND a.session_id=r.selected_session_id
	 WHERE r.instance_id=? AND r.branch_id=? AND r.status='advancing' AND r.settlement_kind='information'
	 AND r.selected_action_kind=? AND a.action_kind=? AND r.selected_session_id=?`,
		b.InstanceID, b.BranchID, actionKind, actionKind, sessionID).Scan(&roundID, &baseline, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return core.NewError(core.CodeUnauthorized, "notice publication is not the selected shared child")
	}
	if err != nil {
		return err
	}
	if b.IdempotencyKey != "shared_action_"+roundID || b.ExpectedHead != baseline {
		return core.NewError(core.CodeUnauthorized, "notice publication differs from selected shared child")
	}
	pinned, err := core.CanonicalJSON(request)
	if err != nil {
		return err
	}
	if string(pinned) != raw {
		return core.NewError(core.CodeUnauthorized, "notice publication payload differs from selected shared child")
	}
	row, err := readRPSharedRoundRow(ctx, conn, roundID)
	if err != nil {
		return err
	}
	if err := authorizeRPSharedParticipant(ctx, conn, row, RPSharedRoundReadRequest{
		PrincipalID: controllerPrincipal, SessionID: sessionID, RoundID: roundID}, true); err != nil {
		return err
	}
	var selectedEntity string
	if err := conn.QueryRowContext(ctx, `SELECT entity_id FROM rp_shared_round_participants WHERE round_id=? AND session_id=?`,
		roundID, sessionID).Scan(&selectedEntity); err != nil {
		return err
	}
	if selectedEntity != speakerID {
		return core.NewError(core.CodeUnauthorized, "notice speaker differs from selected controller")
	}
	return nil
}
