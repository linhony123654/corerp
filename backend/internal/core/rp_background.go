package core

import (
	"strings"
	"time"
)

// Background initialization defines only missing facts for an already
// materialized individual. Existing economic/name/identity evidence wins.
type RPBackgroundRequest struct {
	PrincipalID      string                 `json:"principal_id"`
	InstanceID       string                 `json:"instance_id"`
	BranchID         string                 `json:"branch_id"`
	EntityID         string                 `json:"entity_id"`
	ExpectedHead     int64                  `json:"expected_head"`
	IdempotencyKey   string                 `json:"idempotency_key"`
	AgeMin           int                    `json:"age_min"`
	AgeMax           int                    `json:"age_max"`
	ResidencePlaceID string                 `json:"residence_place_id"`
	InitialPlaceID   string                 `json:"initial_place_id"`
	Schedule         []RPBackgroundSchedule `json:"schedule"`
}

type RPBackgroundSchedule struct {
	EmploymentContractID string `json:"employment_contract_id,omitempty"`
	WorldTime            string `json:"world_time"`
	PlaceID              string `json:"place_id"`
	ActivityCode         string `json:"activity_code"`
}

type RPBackground struct {
	InitialEmployment      []RPOwnEmployment      `json:"initial_employment"`
	Version                string                 `json:"version"`
	EntityID               string                 `json:"entity_id"`
	DisplayName            string                 `json:"display_name"`
	SourceCohortID         string                 `json:"source_cohort_id"`
	MaterializationEventID string                 `json:"materialization_event_id"`
	DefinitionEventID      string                 `json:"definition_event_id"`
	DefinedWorldTime       string                 `json:"defined_world_time"`
	AgeMin                 int                    `json:"age_min"`
	AgeMax                 int                    `json:"age_max"`
	ResidencePlaceID       string                 `json:"residence_place_id"`
	ResidenceSourceEventID string                 `json:"residence_source_event_id"`
	InitialPlaceID         string                 `json:"initial_place_id"`
	Schedule               []RPBackgroundSchedule `json:"initial_schedule"`
	Disposition            RPDisposition          `json:"disposition"`
}

func (r RPBackgroundRequest) Validate() error {
	for _, value := range []string{r.PrincipalID, r.InstanceID, r.BranchID, r.EntityID, r.IdempotencyKey, r.ResidencePlaceID, r.InitialPlaceID} {
		if strings.TrimSpace(value) == "" || len(value) > 256 {
			return NewError(CodeInvalidArgument, "invalid background binding")
		}
	}
	if r.ExpectedHead < 1 || r.ExpectedHead >= MaxJSONSafeInteger || r.AgeMin < 0 || r.AgeMax < r.AgeMin || r.AgeMax > 150 || len(r.Schedule) < 1 || len(r.Schedule) > 16 {
		return NewError(CodeInvalidArgument, "invalid age range, cursor or bounded routine")
	}
	var previous time.Time
	previousPlace := r.InitialPlaceID
	for _, item := range r.Schedule {
		at, err := time.Parse(time.RFC3339, item.WorldTime)
		if err != nil || (!previous.IsZero() && !at.After(previous)) || strings.TrimSpace(item.PlaceID) == "" || item.PlaceID == previousPlace {
			return NewError(CodeInvalidArgument, "routine requires ordered times and actual movement")
		}
		switch item.ActivityCode {
		case "home", "present", "work", "lunch":
		default:
			return NewError(CodeInvalidArgument, "unsupported background routine activity")
		}
		if (item.ActivityCode == "work") != (strings.TrimSpace(item.EmploymentContractID) != "") {
			return NewError(CodeInvalidArgument, "only work routine must reference own employment contract")
		}
		previous, previousPlace = at, item.PlaceID
	}
	return nil
}
