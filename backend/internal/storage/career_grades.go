package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

func (s *Store) DefineCareerGradeScale(ctx context.Context, r core.CareerGradeScaleRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	r.Scale.Grades = append([]string{}, r.Scale.Grades...)
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "DefineCareerGradeScale", r, func(conn *sql.Conn) error {
		return authorizeCareerManager(ctx, conn, b, r.Scale.OrganizationID)
	}, func(conn *sql.Conn, _ careerCommandContext) (CareerFact, func() error, error) {
		if _, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "organization", r.Scale.OrganizationID); err != nil {
			return CareerFact{}, nil, err
		}
		if err := requireNewCareerRecord(ctx, conn, b, "grade_scale", r.Scale.OrganizationID); err != nil {
			return CareerFact{}, nil, err
		}
		// Declaring an order must not orphan previously published positions.
		rows, err := conn.QueryContext(ctx, `SELECT DISTINCT json_extract(payload,'$.posting.grade') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='posting' AND json_extract(payload,'$.organization_id')=?`, b.InstanceID, b.BranchID, r.Scale.OrganizationID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var grade string
			if err := rows.Scan(&grade); err != nil {
				return CareerFact{}, nil, err
			}
			if _, err := r.Scale.Transition(grade, grade); err != nil {
				return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "grade scale excludes an existing position grade")
			}
		}
		if err := rows.Err(); err != nil {
			return CareerFact{}, nil, err
		}
		return CareerFact{Kind: "grade_scale", RecordID: r.Scale.OrganizationID, OrganizationID: r.Scale.OrganizationID, GradeScale: &r.Scale}, nil, nil
	})
}

func validateCareerPostingGrade(ctx context.Context, conn *sql.Conn, b core.CareerBinding, p core.CareerPostingDefinition) error {
	record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "grade_scale", p.OrganizationID)
	if core.HasCode(err, core.CodeNotFound) {
		return nil
	} // Legacy postings remain valid without an inferred order.
	if err != nil {
		return err
	}
	if record.Fact.GradeScale == nil {
		return core.NewError(core.CodeProjectionDiverged, "grade scale event has no scale")
	}
	_, err = record.Fact.GradeScale.Transition(p.Grade, p.Grade)
	return err
}
