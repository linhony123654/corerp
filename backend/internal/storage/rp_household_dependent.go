package storage

import (
	"context"
	"database/sql"
	"strings"
	"unicode"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

type RPHouseholdDependentRequest struct {
	Binding           core.CareerBinding `json:"binding"`
	HouseholdID       string             `json:"household_id"`
	DependentEntityID string             `json:"dependent_entity_id"`
	SupporterEntityID string             `json:"supporter_entity_id"`
	Reason            string             `json:"reason"`
}

type RPHouseholdDependentFact struct {
	Version               string `json:"version"`
	HouseholdID           string `json:"household_id"`
	MembershipID          string `json:"membership_id"`
	DependentEntityID     string `json:"dependent_entity_id"`
	SupporterMembershipID string `json:"supporter_membership_id"`
	Reason                string `json:"reason"`
}

type RPHouseholdDependentRecord = privateFactRecord[RPHouseholdDependentFact]

// The local operator records a care relation between existing Persons. The
// supporter gains no control over the dependent's own accounts or RP session.
func (s *Store) EnrollRPHouseholdDependentLocal(ctx context.Context, r RPHouseholdDependentRequest) (RPHouseholdDependentRecord, error) {
	var empty RPHouseholdDependentRecord
	if err := r.Binding.Validate(); err != nil {
		return empty, err
	}
	if !studioID(r.HouseholdID) || !studioID(r.DependentEntityID) || !studioID(r.SupporterEntityID) || r.DependentEntityID == r.SupporterEntityID ||
		!utf8.ValidString(r.Reason) || r.Reason == "" || len(r.Reason) > 256 || strings.TrimSpace(r.Reason) != r.Reason {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded household dependent and supporter required")
	}
	for _, ch := range r.Reason {
		if unicode.IsControl(ch) {
			return empty, core.NewError(core.CodeInvalidArgument, "household dependency reason contains control characters")
		}
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "EnrollRPHouseholdDependentLocal", r,
		privateFactDomain{"rp_household_dependent", "RPHouseholdDependentEnrolled", `{"authorization":"local-operator-household-dependent-v1"}`},
		func(conn *sql.Conn) error {
			var role string
			if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.Binding.PrincipalID).Scan(&role); err != nil || role != "operator" {
				if err != nil && err != sql.ErrNoRows {
					return err
				}
				return core.NewError(core.CodeUnauthorized, "dependent enrollment requires the local operator")
			}
			return nil
		},
		func(conn *sql.Conn, c privateFactContext) (RPHouseholdDependentFact, func() error, error) {
			var fact RPHouseholdDependentFact
			if err := requireNoActiveRPSharedRound(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			var currency, supporterID string
			err := conn.QueryRowContext(ctx, `SELECT h.currency_id,m.membership_id FROM rp_households h JOIN rp_household_memberships m ON m.household_id=h.household_id AND m.instance_id=h.instance_id AND m.branch_id=h.branch_id
				WHERE h.household_id=? AND h.instance_id=? AND h.branch_id=? AND h.status='active' AND m.member_entity_id=? AND m.member_role='adult' AND m.ended_event_id IS NULL`,
				r.HouseholdID, r.Binding.InstanceID, r.Binding.BranchID, r.SupporterEntityID).Scan(&currency, &supporterID)
			if err != nil {
				return fact, nil, classifyMissing(err, "active household adult supporter")
			}
			var valid int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM materialized_entities e JOIN cohorts source ON source.cohort_id=e.source_cohort_id WHERE e.entity_id=? AND e.status='active' AND e.population_count=1 AND e.currency_id=? AND source.instance_id=? AND source.branch_id=?`, r.DependentEntityID, currency, r.Binding.InstanceID, r.Binding.BranchID).Scan(&valid); err != nil {
				return fact, nil, err
			}
			if valid != 1 {
				return fact, nil, core.NewError(core.CodeNotFound, "active same-currency dependent Person not found")
			}
			var already int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_household_memberships WHERE instance_id=? AND branch_id=? AND member_entity_id=? AND ended_event_id IS NULL`, r.Binding.InstanceID, r.Binding.BranchID, r.DependentEntityID).Scan(&already); err != nil {
				return fact, nil, err
			}
			if already != 0 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "dependent already belongs to an active household")
			}
			fact = RPHouseholdDependentFact{"corerp.household.dependent.v1", r.HouseholdID, "membership_dependent_" + c.EventID, r.DependentEntityID, supporterID, r.Reason}
			return fact, func() error {
				return execAgentOne(ctx, conn, "enroll household dependent", `INSERT INTO rp_household_memberships(membership_id,household_id,instance_id,branch_id,member_entity_id,member_role,source_event_id,started_world_time,supporter_membership_id) VALUES (?,?,?,?,?,'dependent',?,?,?)`, fact.MembershipID, fact.HouseholdID, r.Binding.InstanceID, r.Binding.BranchID, fact.DependentEntityID, c.EventID, c.WorldTime, supporterID)
			}, nil
		})
}
