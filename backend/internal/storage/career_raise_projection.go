package storage

import (
	"context"

	"corerp.local/backend/internal/core"
)

// Contract wage is a projection of initial acceptance and committed activation
// Events. An announced but unactivated future raise must not rebuild early.
// Keep this derived check separate from legacy replay snapshot hashes.
func careerWageProjectionDifferences(ctx context.Context, query replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	rows, err := query.QueryContext(ctx, `SELECT c.contract_id,COALESCE((SELECT json_extract(a.payload,'$.daily_wage_minor') FROM events a WHERE a.instance_id=? AND a.branch_id=? AND a.event_sequence<=? AND a.event_type='CareerEmploymentTermsActivated' AND json_extract(a.payload,'$.contract_id')=c.contract_id ORDER BY a.event_sequence DESC LIMIT 1),json_extract(d.payload,'$.employment.daily_wage_minor')),c.gross_wage_minor,
	 COALESCE((SELECT json_extract(a.payload,'$.position_key') FROM events a WHERE a.instance_id=d.instance_id AND a.branch_id=d.branch_id AND a.event_sequence<=? AND a.event_type='CareerEmploymentTermsActivated' AND json_extract(a.payload,'$.contract_id')=c.contract_id AND json_extract(a.payload,'$.position_key') IS NOT NULL ORDER BY a.event_sequence DESC LIMIT 1),json_extract(d.payload,'$.employment.position_key')),c.position_id,
	 COALESCE((SELECT json_extract(a.payload,'$.contract_status') FROM events a WHERE a.instance_id=d.instance_id AND a.branch_id=d.branch_id AND a.event_sequence<=? AND a.event_type='CareerEmploymentTermsActivated' AND json_extract(a.payload,'$.contract_id')=c.contract_id AND json_extract(a.payload,'$.contract_status') IS NOT NULL ORDER BY a.event_sequence DESC LIMIT 1),'active'),c.status
	 FROM employment_contracts c JOIN events d ON d.event_id=c.definition_event_id WHERE d.instance_id=? AND d.branch_id=? AND d.event_sequence<=? AND d.event_type='RPCareerFactRecorded' AND json_extract(d.payload,'$.employment.contract_id')=c.contract_id ORDER BY c.contract_id`, instance, branch, through, through, through, instance, branch, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var differences []ProjectionDifference
	for rows.Next() {
		difference := ProjectionDifference{Projection: "career_contract_wage"}
		position := ProjectionDifference{Projection: "career_contract_position"}
		status := ProjectionDifference{Projection: "career_contract_status"}
		if err := rows.Scan(&difference.Key, &difference.Expected, &difference.Actual, &position.ExpectedText, &position.ActualText, &status.ExpectedText, &status.ActualText); err != nil {
			return nil, err
		}
		if difference.Expected < 1 || difference.Expected > core.MaxJSONSafeInteger {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid authoritative career wage")
		}
		if difference.Expected != difference.Actual {
			differences = append(differences, difference)
		}
		if position.ExpectedText == "" {
			return nil, core.NewError(core.CodeProjectionDiverged, "missing authoritative position")
		}
		position.Key = difference.Key
		if position.ExpectedText != position.ActualText {
			differences = append(differences, position)
		}
		status.Key = difference.Key
		if status.ExpectedText != "active" && status.ExpectedText != "ended" {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid authoritative employment status")
		}
		if status.ExpectedText != status.ActualText {
			differences = append(differences, status)
		}
	}
	return differences, rows.Err()
}
