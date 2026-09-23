package core

// The announcement text and listeners come from current world facts, not from
// caller-supplied claims or an arbitrary recipient list.
type CareerAnnouncementRequest struct {
	Binding        CareerBinding `json:"binding"`
	AnnouncementID string        `json:"announcement_id"`
	ContractID     string        `json:"contract_id"`
	SpeakerID      string        `json:"speaker_id"`
}

func (r CareerAnnouncementRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	return validateCareerIDs(r.AnnouncementID, r.ContractID, r.SpeakerID)
}
