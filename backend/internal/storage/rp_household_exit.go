package storage

import (
	"context"
	"database/sql"
	"strings"
	"unicode"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

type RPHouseholdMemberExitRequest struct {
	Binding     core.CareerBinding `json:"binding"`
	SessionID   string             `json:"session_id"`
	HouseholdID string             `json:"household_id"`
	Reason      string             `json:"reason"`
}

type RPHouseholdMemberExitFact struct {
	Version        string `json:"version"`
	HouseholdID    string `json:"household_id"`
	MembershipID   string `json:"membership_id"`
	MemberEntityID string `json:"member_entity_id"`
	Reason         string `json:"reason"`
}

type RPHouseholdMemberExitRecord = privateFactRecord[RPHouseholdMemberExitFact]

// Leaving ends one sourced membership interval. It neither deletes the Person
// nor transfers that person's contractual share or wallet authority.
func (s *Store) LeaveRPHousehold(ctx context.Context, r RPHouseholdMemberExitRequest) (RPHouseholdMemberExitRecord, error) {
	var empty RPHouseholdMemberExitRecord
	if err := r.Binding.Validate(); err != nil {
		return empty, err
	}
	if !studioID(r.SessionID) || !studioID(r.HouseholdID) || strings.TrimSpace(r.SessionID) != r.SessionID || strings.TrimSpace(r.HouseholdID) != r.HouseholdID ||
		!utf8.ValidString(r.Reason) || strings.TrimSpace(r.Reason) != r.Reason || r.Reason == "" || len(r.Reason) > 256 {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded household exit reason and session required")
	}
	for _, ch := range r.Reason {
		if unicode.IsControl(ch) {
			return empty, core.NewError(core.CodeInvalidArgument, "household exit reason contains control characters")
		}
	}
	// A lost response remains recoverable by the original session principal;
	// fresh exits still need live control, a current observation and membership.
	replayAuth := func(conn *sql.Conn) error {
		session, err := loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
		if err != nil {
			return err
		}
		if session.InstanceID != r.Binding.InstanceID || session.BranchID != r.Binding.BranchID {
			return core.NewError(core.CodeNotFound, "session is outside requested world branch")
		}
		return nil
	}
	return executePrivateFactCommandWithOptions(s, ctx, r.Binding, "LeaveRPHousehold", r,
		privateFactDomain{"rp_household_exit", "RPHouseholdMemberLeft", `{"authorization":"current-member-self-exit-v1"}`},
		privateFactOptions{replayAuthorize: replayAuth},
		func(conn *sql.Conn) error {
			if err := replayAuth(conn); err != nil {
				return err
			}
			session, err := loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return err
			}
			if session.Status != "active" {
				return core.NewError(core.CodeBranchConflict, "household exit requires active session")
			}
			if err := requireCurrentRPSession(ctx, conn, session); err != nil {
				return err
			}
			if err := authorizeRPControl(ctx, conn, r.Binding.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
				return err
			}
			if session.ObservationCursor != r.Binding.ExpectedHead {
				return core.NewError(core.CodeBranchConflict, "observe current world before leaving household")
			}
			return nil
		},
		func(conn *sql.Conn, c privateFactContext) (RPHouseholdMemberExitFact, func() error, error) {
			var fact RPHouseholdMemberExitFact
			if err := requireNoActiveRPSharedRound(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			session, err := loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return fact, nil, err
			}
			var membershipID string
			err = conn.QueryRowContext(ctx, `SELECT m.membership_id FROM rp_household_memberships m JOIN rp_households h ON h.household_id=m.household_id AND h.instance_id=m.instance_id AND h.branch_id=m.branch_id
				WHERE h.household_id=? AND h.instance_id=? AND h.branch_id=? AND h.status='active' AND m.member_entity_id=? AND m.member_role='adult' AND m.ended_event_id IS NULL`,
				r.HouseholdID, r.Binding.InstanceID, r.Binding.BranchID, session.ControlledEntityID).Scan(&membershipID)
			if err != nil {
				return fact, nil, classifyMissing(err, "active adult household membership")
			}
			var adults int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_household_memberships WHERE household_id=? AND instance_id=? AND branch_id=? AND member_role='adult' AND ended_event_id IS NULL`, r.HouseholdID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&adults); err != nil {
				return fact, nil, err
			}
			if adults < 2 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "last active adult requires explicit household closure")
			}
			var dependents int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_household_memberships WHERE household_id=? AND supporter_membership_id=? AND member_role='dependent' AND ended_event_id IS NULL`, r.HouseholdID, membershipID).Scan(&dependents); err != nil {
				return fact, nil, err
			}
			if dependents != 0 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "transfer active dependent support before adult exit")
			}
			fact = RPHouseholdMemberExitFact{"corerp.household.member_exit.v1", r.HouseholdID, membershipID, session.ControlledEntityID, r.Reason}
			return fact, func() error {
				return execAgentOne(ctx, conn, "end household membership", `UPDATE rp_household_memberships SET ended_event_id=?,ended_world_time=? WHERE membership_id=? AND household_id=? AND member_entity_id=? AND ended_event_id IS NULL`, c.EventID, c.WorldTime, membershipID, r.HouseholdID, session.ControlledEntityID)
			}, nil
		})
}
