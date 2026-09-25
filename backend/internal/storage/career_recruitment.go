package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

func (s *Store) DefineCareerOrganization(ctx context.Context, r core.CareerOrganizationRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b, o := r.Binding, r.Organization
	authorize := func(conn *sql.Conn) error {
		// Initial world construction only: the creator's grant must match the
		// Cohort that actually funded this scoped employer, not an unrelated one.
		var count int
		err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_economic_actors a JOIN events e ON e.event_id=a.definition_event_id AND e.instance_id=a.instance_id AND e.branch_id=a.branch_id
		 JOIN capability_grants g ON g.instance_id=a.instance_id AND g.branch_id=a.branch_id AND g.subject_id=json_extract(e.payload,'$.cohort_id')
		 JOIN principals p ON p.principal_id=g.principal_id WHERE a.actor_id=? AND a.instance_id=? AND a.branch_id=? AND a.kind='employer'
		 AND p.principal_id=? AND p.principal_type='creator' AND p.status='active' AND g.capability_id='world.cohort.materialize' AND g.status='active'`, o.OrganizationID, b.InstanceID, b.BranchID, b.PrincipalID).Scan(&count)
		if err != nil {
			return err
		}
		if count != 1 {
			return core.NewError(core.CodeUnauthorized, "organization definition requires its source Cohort's creator authority")
		}
		return nil
	}
	return s.executeCareerCommand(ctx, b, "DefineCareerOrganization", r, authorize, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "organization", o.OrganizationID); err != nil {
			return CareerFact{}, nil, err
		}
		var valid int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM principals WHERE principal_id=? AND status='active'`, o.ManagerPrincipalID).Scan(&valid); err != nil {
			return CareerFact{}, nil, err
		}
		if valid != 1 {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "organization manager must be an active principal")
		}
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=? AND place_kind='work' AND status='active'`, o.WorkplaceID, b.InstanceID, b.BranchID).Scan(&valid); err != nil {
			return CareerFact{}, nil, err
		}
		if valid != 1 {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "organization requires an existing scoped workplace")
		}
		org := CareerOrganizationFact{Definition: o}
		if err := conn.QueryRowContext(ctx, `SELECT a.cash_account_id,c.currency_id,a.definition_event_id FROM m2_economic_actors a JOIN accounts c ON c.account_id=a.cash_account_id AND c.owner_id=a.actor_id AND c.account_type='asset' AND c.closed_by_event_id IS NULL WHERE a.actor_id=? AND a.instance_id=? AND a.branch_id=? AND a.kind='employer'`, o.OrganizationID, b.InstanceID, b.BranchID).Scan(&org.CashAccountID, &org.CurrencyID, &org.SourceActorEventID); err != nil {
			return CareerFact{}, nil, classifyMissing(err, "funded organization account")
		}
		fact := CareerFact{Kind: "organization", RecordID: o.OrganizationID, OrganizationID: o.OrganizationID, Organization: &org}
		return fact, func() error {
			if _, err := conn.ExecContext(ctx, `INSERT INTO economic_entities(entity_id,entity_kind,display_name,account_id,private_finances) VALUES (?,'enterprise',?,?,1) ON CONFLICT(entity_id) DO NOTHING`, o.OrganizationID, o.DisplayName, org.CashAccountID); err != nil {
				return err
			}
			var matching int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM economic_entities WHERE entity_id=? AND entity_kind='enterprise' AND account_id=? AND private_finances=1`, o.OrganizationID, org.CashAccountID).Scan(&matching); err != nil {
				return err
			}
			if matching != 1 {
				return core.NewError(core.CodeProjectionDiverged, "organization economic identity differs")
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO capability_definitions(capability_id,description,policy_version) VALUES (?,'Manage one organization career lifecycle','career-v1') ON CONFLICT(capability_id) DO NOTHING`, careerManageCapability); err != nil {
				return err
			}
			_, err := conn.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,?,?,?,?,'[]','active',?)`, "grant_"+c.EventID, o.ManagerPrincipalID, careerManageCapability, b.InstanceID, b.BranchID, o.OrganizationID, c.EventID)
			return err
		}, nil
	})
}

func (s *Store) PostCareerPosition(ctx context.Context, r core.CareerPostingRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	// Copy the caller's slice before storing/hash construction.
	r.Posting.RequiredQualifications = append([]string{}, r.Posting.RequiredQualifications...)
	r.Posting.RequiredCredentials = append([]core.CredentialRequirement(nil), r.Posting.RequiredCredentials...)
	r.Posting.Capabilities = append([]string(nil), r.Posting.Capabilities...)
	b, p := r.Binding, r.Posting
	return s.executeCareerCommand(ctx, b, "PostCareerPosition", r, func(conn *sql.Conn) error {
		return authorizeCareerManager(ctx, conn, b, p.OrganizationID)
	}, func(conn *sql.Conn, _ careerCommandContext) (CareerFact, func() error, error) {
		if _, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "organization", p.OrganizationID); err != nil {
			return CareerFact{}, nil, err
		}
		if err := requireNewCareerRecord(ctx, conn, b, "posting", p.PositionID); err != nil {
			return CareerFact{}, nil, err
		}
		if err := validateCareerPostingGrade(ctx, conn, b, p); err != nil {
			return CareerFact{}, nil, err
		}
		return CareerFact{Kind: "posting", RecordID: p.PositionID, OrganizationID: p.OrganizationID, Posting: &p}, nil, nil
	})
}

func (s *Store) ApplyForCareerPosition(ctx context.Context, r core.CareerApplicationRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "ApplyForCareerPosition", r, func(conn *sql.Conn) error {
		return authorizeCareerCandidate(ctx, conn, b, r.CandidateID)
	}, func(conn *sql.Conn, _ careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "application", r.ApplicationID); err != nil {
			return CareerFact{}, nil, err
		}
		posting, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "posting", r.PositionID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if posting.Fact.Posting == nil {
			return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "posting lacks terms")
		}
		var worldTime string
		if err := conn.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, b.InstanceID, b.BranchID).Scan(&worldTime); err != nil {
			return CareerFact{}, nil, err
		}
		if err := requireCareerCredentials(ctx, conn, b, r.CandidateID, posting.Fact.Posting.RequiredCredentials, worldTime); err != nil {
			return CareerFact{}, nil, err
		}
		rows, err := conn.QueryContext(ctx, `SELECT DISTINCT json_extract(payload,'$.record_id') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='application' AND json_extract(payload,'$.application.position_id')=? AND json_extract(payload,'$.application.candidate_id')=?`, b.InstanceID, b.BranchID, r.PositionID, r.CandidateID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		var priorIDs []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return CareerFact{}, nil, err
			}
			priorIDs = append(priorIDs, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return CareerFact{}, nil, err
		}
		rows.Close()
		for _, id := range priorIDs {
			prior, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "application", id)
			if err != nil {
				return CareerFact{}, nil, err
			}
			if prior.Fact.Application == nil || prior.Fact.Application.Status == "submitted" {
				return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "candidate already applied to this position")
			}
		}
		var source string
		if err := conn.QueryRowContext(ctx, `SELECT m.materialize_event_id FROM materialized_entities n JOIN cohort_materializations m ON m.materialization_id=n.materialization_id WHERE n.entity_id=?`, r.CandidateID).Scan(&source); err != nil {
			return CareerFact{}, nil, err
		}
		application := CareerApplicationFact{ApplicationID: r.ApplicationID, PositionID: r.PositionID, CandidateID: r.CandidateID, Statement: r.Statement, PostingEventID: posting.EventID, CandidateSourceID: source, Status: "submitted"}
		if r.ReferralID != "" {
			referral, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "referral", r.ReferralID)
			if err != nil {
				return CareerFact{}, nil, err
			}
			if referral.Fact.Referral == nil || referral.Fact.CandidateID != r.CandidateID || referral.Fact.Referral.PositionID != r.PositionID || referral.Fact.Referral.PostingEventID != posting.EventID {
				return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "referral does not match this candidate and posting")
			}
			application.ReferralEventID = referral.EventID
		}
		return CareerFact{Kind: "application", RecordID: r.ApplicationID, OrganizationID: posting.Fact.OrganizationID, CandidateID: r.CandidateID, Application: &application}, nil, nil
	})
}

type CareerPostingView struct {
	AvailableSlots int                          `json:"available_slots"`
	Posting        core.CareerPostingDefinition `json:"posting"`
	SourceEventID  string                       `json:"source_event_id"`
	EventSequence  int64                        `json:"event_sequence"`
}

type CareerMarket struct {
	Postings   []CareerPostingView `json:"postings"`
	NextCursor int64               `json:"next_cursor"`
}

// Discovery is an explicit public job-board read by a controlled individual;
// it returns no applications, manager grants, cash accounts or private facts.
func (s *Store) DiscoverCareerPositions(ctx context.Context, principalID, instanceID, branchID, candidateID string, after int64) (CareerMarket, error) {
	view := CareerMarket{Postings: []CareerPostingView{}}
	if after < 0 || after >= core.MaxJSONSafeInteger {
		return view, core.NewError(core.CodeInvalidArgument, "invalid career market cursor")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return view, err
	}
	defer tx.Rollback(ctx)
	b := core.CareerBinding{PrincipalID: principalID, InstanceID: instanceID, BranchID: branchID}
	if err := authorizeCareerCandidate(ctx, tx.conn, b, candidateID); err != nil {
		return view, err
	}
	rows, err := tx.conn.QueryContext(ctx, `SELECT event_id,event_sequence,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='posting' AND event_sequence>? ORDER BY event_sequence LIMIT 100`, instanceID, branchID, after)
	if err != nil {
		return view, err
	}
	defer rows.Close()
	for rows.Next() {
		var item CareerPostingView
		var raw string
		var fact CareerFact
		if err := rows.Scan(&item.SourceEventID, &item.EventSequence, &raw); err != nil {
			return CareerMarket{}, err
		}
		if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Posting == nil {
			return CareerMarket{}, core.NewError(core.CodeProjectionDiverged, "invalid career posting fact")
		}
		item.Posting = *fact.Posting
		view.Postings = append(view.Postings, item)
		view.NextCursor = item.EventSequence
	}
	if err := rows.Err(); err != nil {
		return CareerMarket{}, err
	}
	rows.Close()
	for i := range view.Postings {
		key, err := careerPositionKey(instanceID, branchID, view.Postings[i].Posting.PositionID)
		if err != nil {
			return CareerMarket{}, err
		}
		occupied, err := careerPositionOccupancy(ctx, tx.conn, key)
		if err != nil {
			return CareerMarket{}, err
		}
		if occupied > view.Postings[i].Posting.Capacity {
			return CareerMarket{}, core.NewError(core.CodeProjectionDiverged, "position capacity exceeded")
		}
		view.Postings[i].AvailableSlots = view.Postings[i].Posting.Capacity - occupied
	}
	return view, nil
}

func (s *Store) ReadCareerApplication(ctx context.Context, principalID, instanceID, branchID, applicationID string) (CareerRecord, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return CareerRecord{}, err
	}
	defer tx.Rollback(ctx)
	record, err := readCareerRecord(ctx, tx.conn, instanceID, branchID, "application", applicationID)
	if err != nil {
		return CareerRecord{}, err
	}
	if record.Fact.Application == nil {
		return CareerRecord{}, core.NewError(core.CodeProjectionDiverged, "invalid career application fact")
	}
	b := core.CareerBinding{PrincipalID: principalID, InstanceID: instanceID, BranchID: branchID}
	if err := authorizeCareerManager(ctx, tx.conn, b, record.Fact.OrganizationID); err != nil {
		if !core.HasCode(err, core.CodeUnauthorized) {
			return CareerRecord{}, err
		}
		if err := authorizeCareerCandidate(ctx, tx.conn, b, record.Fact.Application.CandidateID); err != nil {
			return CareerRecord{}, err
		}
	}
	return record, nil
}
