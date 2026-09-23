package storage

import (
	"context"
	"database/sql"
	"errors"

	"corerp.local/backend/internal/core"
)

const rpLifeSetupTime = "2026-09-22T07:03:00Z"

type RPLifeSetupResult struct {
	Economy      M2EconomicSetupResult `json:"economy"`
	Participants RPPlaySetupResult     `json:"participants"`
	Travel       RPTravelSetupResult   `json:"travel"`
	Routine      AgentRoutineResult    `json:"routine"`
}

// PrepareRPLifeDemo is an explicit, resumable demo composition, not a startup
// hook or an atomic world conversion. Existing minimal RP demos remain unchanged.
// Establish the funded employment contract before splitting its participants.
func (s *Store) PrepareRPLifeDemo(ctx context.Context) (RPLifeSetupResult, error) {
	var result RPLifeSetupResult
	if _, err := s.BootstrapM2AgentDemo(ctx); err != nil {
		return result, err
	}
	var err error
	if result.Economy, err = s.PrepareM2EconomicDemo(ctx); err != nil {
		return result, err
	}
	for _, command := range []core.MaterializeCohortCommand{
		// No earned wage claim exists before the first accrual. Future wages
		// follow participation; they are not fabricated opening receivables.
		m2AgentMaterialization("rp_cai", M2RPNPCID, "Cai", 1, 400, 3, 0, 60, 0, "2026-09-22T07:01:00Z"),
		m2AgentMaterialization("rp_lin", M2RPPlayerID, "Lin", 1, 300, 3, 0, 45, 0, "2026-09-22T07:02:00Z"),
	} {
		// Retry against the originally committed head, not today's branch head.
		err := s.db.QueryRowContext(ctx, `SELECT expected_head FROM commands WHERE command_id=?`, command.CommandID).Scan(&command.ExpectedHead)
		if errors.Is(err, sql.ErrNoRows) {
			err = s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&command.ExpectedHead)
		}
		if err != nil {
			return result, core.WrapError(core.CodeStorageFailure, "read life materialization cursor", err)
		}
		if _, err := s.MaterializeCohort(ctx, command); err != nil {
			return result, err
		}
	}
	if result.Participants, err = s.setupRPParticipantsAt(ctx, rpLifeSetupTime); err != nil {
		return result, err
	}
	if result.Travel, err = s.setupRPTravelAt(ctx, rpLifeSetupTime); err != nil {
		return result, err
	}
	result.Routine, err = s.DefineM2AgentRoutine(ctx, core.AgentRoutineRequest{
		PrincipalID: "principal_creator", CapabilityID: "world.agent.run",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, Days: 30,
	})
	return result, err
}
