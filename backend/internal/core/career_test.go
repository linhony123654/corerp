package core

import "testing"

func TestCareerPostingValidationAndBindingHash(t *testing.T) {
	valid := CareerPostingRequest{Binding: CareerBinding{PrincipalID: "manager", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "key"}, Posting: CareerPostingDefinition{PositionID: "position", OrganizationID: "org", Title: "Assistant", OccupationID: "operations", Grade: "entry", Capacity: 1, DailyWageMinor: 12, RequiredQualifications: []string{"training"}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"capacity", "wage", "head", "duplicate_requirement", "blank_grade", "whitespace_id", "creator_capability", "duplicate_capability"} {
		t.Run(name, func(t *testing.T) {
			r := valid
			switch name {
			case "capacity":
				r.Posting.Capacity = 0
			case "wage":
				r.Posting.DailyWageMinor = MaxJSONSafeInteger + 1
			case "head":
				r.Binding.ExpectedHead = MaxJSONSafeInteger
			case "duplicate_requirement":
				r.Posting.RequiredQualifications = []string{"training", "training"}
			case "blank_grade":
				r.Posting.Grade = ""
			case "whitespace_id":
				r.Posting.PositionID = " position"
			case "creator_capability":
				r.Posting.Capabilities = []string{"world.cohort.materialize"}
			case "duplicate_capability":
				r.Posting.Capabilities = []string{CareerPositionManageCapability, CareerPositionManageCapability}
			}
			if !HasCode(r.Validate(), CodeInvalidArgument) {
				t.Fatal("invalid posting accepted")
			}
		})
	}
	original, err := HashJSON(valid)
	if err != nil {
		t.Fatal(err)
	}
	changed := valid
	changed.Binding.PrincipalID = "another_manager"
	other, err := HashJSON(changed)
	if err != nil || original == other {
		t.Fatal("canonical request hash lost the principal binding")
	}
}
