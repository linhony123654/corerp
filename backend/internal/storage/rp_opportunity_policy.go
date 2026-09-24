package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"corerp.local/backend/internal/core"
)

// Optional source families default off. Installation is
// explicit and branch-scoped; old worlds remain unchanged until configured.
type RPOpportunityPolicy struct {
	VisitBasisPoints     int    `json:"visit_basis_points,omitempty"`
	RareVisitBasisPoints int    `json:"rare_visit_basis_points,omitempty"`
	CommunityBasisPoints int    `json:"community_basis_points,omitempty"`
	WarmEnabled          bool   `json:"warm_enabled,omitempty"`
	WorkBasisPoints      int    `json:"work_basis_points,omitempty"`
	StreamSeed           string `json:"stream_seed"`
	ContactBasisPoints   int    `json:"contact_basis_points"`
	CooldownHours        int    `json:"cooldown_hours"`
	HistoryHours         int    `json:"history_hours"`
}

func (p RPOpportunityPolicy) validate() error {
	if _, err := core.RPOpportunityProbability(p.VisitBasisPoints, false, core.RPOpportunityPressure{}); err != nil {
		return err
	}
	if _, err := core.RPOpportunityProbability(p.RareVisitBasisPoints, true, core.RPOpportunityPressure{}); err != nil {
		return err
	}
	if p.RareVisitBasisPoints > 0 && p.HistoryHours < 168 {
		return core.NewError(core.CodeInvalidArgument, "rare visits require at least seven days of source history")
	}
	if strings.TrimSpace(p.StreamSeed) == "" || len(p.StreamSeed) > 256 || p.CooldownHours < 1 || p.CooldownHours > 168 || p.HistoryHours < p.CooldownHours || p.HistoryHours > 720 {
		return core.NewError(core.CodeInvalidArgument, "opportunity policy requires bounded seed, cooldown and history")
	}
	if _, err := core.RPOpportunityProbability(p.WorkBasisPoints, false, core.RPOpportunityPressure{}); err != nil {
		return err
	}
	if _, err := core.RPOpportunityProbability(p.CommunityBasisPoints, false, core.RPOpportunityPressure{}); err != nil {
		return err
	}
	_, err := core.RPOpportunityProbability(p.ContactBasisPoints, false, core.RPOpportunityPressure{})
	return err
}

type OpportunityPolicyRequest struct {
	Binding core.CareerBinding  `json:"binding"`
	Policy  RPOpportunityPolicy `json:"policy"`
}

type OpportunityPolicyFact struct {
	Version              string              `json:"version"`
	BuilderSourceEventID string              `json:"builder_source_event_id"`
	Policy               RPOpportunityPolicy `json:"policy"`
}

type OpportunityPolicyRecord = privateFactRecord[OpportunityPolicyFact]

// One immutable installation prevents fresh keys from resetting the stream or
// cooldown. Future policy evolution must preserve receipts across revisions.
func (s *Store) DefineRPOpportunityPolicy(ctx context.Context, r OpportunityPolicyRequest) (OpportunityPolicyRecord, error) {
	if err := r.Policy.validate(); err != nil {
		return OpportunityPolicyRecord{}, err
	}
	var builder string
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPOpportunityPolicy", r,
		privateFactDomain{"opportunity", "RPOpportunityPolicyDefined", `{"authorization":"opportunity-builder-v1"}`},
		func(conn *sql.Conn) error {
			var err error
			builder, err = authorizeCultureBuilder(ctx, conn, r.Binding)
			return err
		}, func(conn *sql.Conn, _ privateFactContext) (OpportunityPolicyFact, func() error, error) {
			var count int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPOpportunityPolicyDefined'`, r.Binding.InstanceID, r.Binding.BranchID).Scan(&count); err != nil {
				return OpportunityPolicyFact{}, nil, err
			}
			if count != 0 {
				return OpportunityPolicyFact{}, nil, core.NewError(core.CodeBranchConflict, "opportunity stream already installed")
			}
			return OpportunityPolicyFact{Version: "corerp.opportunity.policy.v1", BuilderSourceEventID: builder, Policy: r.Policy}, nil, nil
		})
}

// Absence is explicit: sql.ErrNoRows means no opportunity layer is enabled.
// The stream is internal simulation configuration, never an NPC knowledge fact.
func readRPOpportunityPolicy(ctx context.Context, conn *sql.Conn, instanceID, branchID string) (OpportunityPolicyRecord, error) {
	var record OpportunityPolicyRecord
	var raw string
	err := conn.QueryRowContext(ctx, `SELECT event_id,event_sequence,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPOpportunityPolicyDefined' ORDER BY event_sequence LIMIT 1`, instanceID, branchID).Scan(&record.EventID, &record.EventSequence, &record.WorldTime, &raw)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal([]byte(raw), &record.Fact); err != nil {
		return record, err
	}
	if record.Fact.Version != "corerp.opportunity.policy.v1" || record.Fact.BuilderSourceEventID == "" {
		return record, core.NewError(core.CodeProjectionDiverged, "invalid opportunity policy source")
	}
	if err := record.Fact.Policy.validate(); err != nil {
		return record, err
	}
	return record, nil
}
