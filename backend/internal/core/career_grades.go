package core

// Grades are ordered from lowest to highest within one organization only.
// This declaration confers neither qualifications nor authority on a person.
type CareerGradeScale struct {
	OrganizationID string   `json:"organization_id"`
	Grades         []string `json:"grades"`
}

type CareerGradeScaleRequest struct {
	Binding CareerBinding    `json:"binding"`
	Scale   CareerGradeScale `json:"scale"`
}

func (s CareerGradeScale) Validate() error {
	if err := validateCareerIDs(s.OrganizationID); err != nil {
		return err
	}
	if len(s.Grades) < 2 || len(s.Grades) > 16 {
		return NewError(CodeInvalidArgument, "grade scale requires two to sixteen ordered grades")
	}
	seen := map[string]bool{}
	for _, grade := range s.Grades {
		if err := validateCareerIDs(grade); err != nil {
			return err
		}
		if seen[grade] {
			return NewError(CodeInvalidArgument, "duplicate organization grade")
		}
		seen[grade] = true
	}
	return nil
}

func (r CareerGradeScaleRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	return r.Scale.Validate()
}

// Transition compares declared order, never labels, wages, or experience.
func (s CareerGradeScale) Transition(from, to string) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	oldRank, newRank := -1, -1
	for rank, grade := range s.Grades {
		if grade == from {
			oldRank = rank
		}
		if grade == to {
			newRank = rank
		}
	}
	if oldRank < 0 || newRank < 0 {
		return "", NewError(CodeInvalidArgument, "position grade is absent from organization scale")
	}
	if newRank > oldRank {
		return "promotion", nil
	}
	if newRank < oldRank {
		return "demotion", nil
	}
	return "transfer", nil
}
