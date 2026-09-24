package core

// Public shelf availability at the observer's current place. Exact inventories,
// supplier terms and store finances are not part of this observation.
type RPStoreAvailability struct {
	PlaceID                 string `json:"place_id"`
	StoreActorID            string `json:"store_actor_id"`
	SKUID                   string `json:"sku_id"`
	Available               bool   `json:"available"`
	StorefrontSourceEventID string `json:"storefront_source_event_id"`
	StockSourceEventID      string `json:"stock_source_event_id"`
}

type RPStoreOpportunityContext struct {
	Store    RPStoreAvailability `json:"store"`
	Selected bool                `json:"selected"`
}

func RPSelectedStoreShortage(input RPDecisionInput) bool {
	for _, opportunity := range input.StoreOpportunities {
		if !opportunity.Selected || opportunity.Store.Available || opportunity.Store.PlaceID != input.PlaceID || opportunity.Store.StockSourceEventID == "" || opportunity.Store.StorefrontSourceEventID == "" {
			continue
		}
		for _, shelf := range input.Stores {
			if shelf == opportunity.Store {
				return true
			}
		}
	}
	return false
}
