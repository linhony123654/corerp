package core

// This local fallback chooses voluntary overtime, not hiring eligibility.
// Its evidence is private to the employee; execution still validates authority,
// current terms, leave, schedule conflicts and the wage ceiling.
type CareerOvertimeChoice struct {
	Version              string   `json:"version"`
	Decision             string   `json:"decision"`
	ReasonCode           string   `json:"reason_code"`
	CounterpartyEntityID string   `json:"counterparty_entity_id,omitempty"`
	SourceEventIDs       []string `json:"source_event_ids"`
}

// A known coworker is an attributed announcement the employee actually heard,
// not access to the organization's private roster or the coworker's schedule.
type CareerKnownCoworker struct {
	EntityID      string
	SourceEventID string
}

func ChooseCareerOvertime(input RPDecisionInput, manager string, coworkers ...CareerKnownCoworker) CareerOvertimeChoice {
	choice := CareerOvertimeChoice{Version: "corerp.career.overtime_choice.v1", Decision: "decline", ReasonCode: "prefer_free_time", CounterpartyEntityID: manager, SourceEventIDs: []string{}}
	if input.Life == nil {
		return choice
	}
	life := input.Life
	if len(coworkers) > 0 {
		choice.Version = "corerp.career.overtime_choice.v2"
	}
	var trustedManager *RPRelationship
	for _, r := range life.Relationships {
		if manager == "" || r.SubjectEntityID != manager || len(r.SourceEventIDs) == 0 {
			continue
		}
		if r.Tension > 2+life.Disposition.Patience || r.Trust < 0 {
			choice.ReasonCode = "preserve_boundaries_with_manager"
			choice.SourceEventIDs = append(choice.SourceEventIDs, r.SourceEventIDs...)
			if life.Disposition.SourceEventID != "" {
				choice.SourceEventIDs = append(choice.SourceEventIDs, life.Disposition.SourceEventID)
			}
			return choice
		}
		if r.Trust > 0 && r.Affinity > 0 {
			copy := r
			trustedManager = &copy
		}
	}
	var trustedCoworker *CareerOvertimeChoice
	for _, coworker := range coworkers {
		if coworker.EntityID == "" || coworker.EntityID == manager || coworker.EntityID == input.NPCEntityID || coworker.SourceEventID == "" {
			continue
		}
		for _, r := range life.Relationships {
			if r.SubjectEntityID != coworker.EntityID || len(r.SourceEventIDs) == 0 {
				continue
			}
			peerChoice := CareerOvertimeChoice{Version: choice.Version, Decision: "decline", ReasonCode: "avoid_strained_workplace_relationship", CounterpartyEntityID: coworker.EntityID, SourceEventIDs: append([]string{coworker.SourceEventID}, r.SourceEventIDs...)}
			if r.Tension > 2+life.Disposition.Patience || r.Trust < 0 {
				if life.Disposition.SourceEventID != "" {
					peerChoice.SourceEventIDs = append(peerChoice.SourceEventIDs, life.Disposition.SourceEventID)
				}
				return peerChoice
			}
			if r.Trust > 0 && r.Affinity > 0 && trustedCoworker == nil {
				peerChoice.Decision, peerChoice.ReasonCode = "accept", "comfortable_with_known_coworker"
				trustedCoworker = &peerChoice
			}
		}
	}
	if trustedManager != nil {
		choice.Decision, choice.ReasonCode = "accept", "cooperate_with_trusted_manager"
		choice.SourceEventIDs = append(choice.SourceEventIDs, trustedManager.SourceEventIDs...)
		return choice
	}
	if trustedCoworker != nil {
		return *trustedCoworker
	}
	for _, need := range life.Needs {
		if need.Code == "cash_security" && need.Urgency == "high" && len(need.SourceEventIDs) > 0 {
			choice.Decision, choice.ReasonCode = "accept", "seek_optional_earned_income"
			choice.SourceEventIDs = append(choice.SourceEventIDs, need.SourceEventIDs...)
			return choice
		}
	}
	return choice
}
