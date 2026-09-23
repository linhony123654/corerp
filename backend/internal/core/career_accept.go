package core

type CareerOfferAcceptRequest struct {
	Binding          CareerBinding `json:"binding"`
	OfferID          string        `json:"offer_id"`
	AfterWorkPlaceID string        `json:"after_work_place_id"`
}

func (r CareerOfferAcceptRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	return validateCareerIDs(r.OfferID, r.AfterWorkPlaceID)
}
