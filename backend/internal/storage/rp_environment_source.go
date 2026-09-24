package storage

import (
	"context"
	"database/sql"
	"strings"

	"corerp.local/backend/internal/core"
)

// A source configures possible local rain; it is not an observed weather fact.
// Shared condition draws must later use place/window, never actor/session IDs.
type RPEnvironmentSource struct {
	PlaceID         string `json:"place_id"`
	RainBasisPoints int    `json:"rain_basis_points"`
	CooldownHours   int    `json:"cooldown_hours"`
}

func (s RPEnvironmentSource) validate() error {
	if strings.TrimSpace(s.PlaceID) == "" || len(s.PlaceID) > 256 || s.CooldownHours < 1 || s.CooldownHours > 168 {
		return core.NewError(core.CodeInvalidArgument, "environment source requires bounded place and cooldown")
	}
	_, err := core.RPOpportunityProbability(s.RainBasisPoints, false, core.RPOpportunityPressure{})
	return err
}

type EnvironmentSourceRequest struct {
	Binding core.CareerBinding  `json:"binding"`
	Source  RPEnvironmentSource `json:"source"`
}

type EnvironmentSourceFact struct {
	Version              string              `json:"version"`
	BuilderSourceEventID string              `json:"builder_source_event_id"`
	PlaceSourceEventID   string              `json:"place_source_event_id"`
	PolicyEventID        string              `json:"policy_event_id"`
	Source               RPEnvironmentSource `json:"source"`
}

type EnvironmentSourceRecord = privateFactRecord[EnvironmentSourceFact]

func (s *Store) DefineRPEnvironmentSource(ctx context.Context, r EnvironmentSourceRequest) (EnvironmentSourceRecord, error) {
	if err := r.Source.validate(); err != nil {
		return EnvironmentSourceRecord{}, err
	}
	var builder string
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPEnvironmentSource", r,
		privateFactDomain{"environment", "RPEnvironmentSourceDefined", `{"authorization":"environment-builder-v1"}`},
		func(conn *sql.Conn) error {
			var err error
			builder, err = authorizeCultureBuilder(ctx, conn, r.Binding)
			return err
		}, func(conn *sql.Conn, _ privateFactContext) (EnvironmentSourceFact, func() error, error) {
			var empty EnvironmentSourceFact
			policy, err := readRPOpportunityPolicy(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID)
			if err != nil {
				return empty, nil, classifyMissing(err, "installed opportunity stream")
			}
			var placeSource string
			if err := conn.QueryRowContext(ctx, `SELECT definition_event_id FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=? AND status='active'`, r.Source.PlaceID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&placeSource); err != nil {
				return empty, nil, classifyMissing(err, "actual environment place")
			}
			var exists int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPEnvironmentSourceDefined' AND json_extract(payload,'$.source.place_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.Source.PlaceID).Scan(&exists); err != nil {
				return empty, nil, err
			}
			if exists != 0 {
				return empty, nil, core.NewError(core.CodeBranchConflict, "environment source already installed for place")
			}
			return EnvironmentSourceFact{Version: "corerp.environment.source.v1", BuilderSourceEventID: builder, PlaceSourceEventID: placeSource, PolicyEventID: policy.EventID, Source: r.Source}, nil, nil
		})
}
