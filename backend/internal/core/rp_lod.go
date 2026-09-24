package core

import "sort"

const RPHotInitiativeLimit = 16

// SelectRPHotInitiatives bounds model work in the current scene without
// permanently favoring low entity IDs. Last decisions come from accepted
// actor/world Events, not session-local counters. The committed wait pins the
// chosen roster; this is not a new character state or background simulation.
func SelectRPHotInitiatives(present []string, lastDecisionSequence map[string]int64) ([]string, error) {
	seen := make(map[string]bool, len(present))
	roster := make([]string, 0, len(present))
	for _, id := range present {
		if id == "" || seen[id] || lastDecisionSequence[id] < 0 {
			return nil, NewError(CodeInvalidArgument, "HOT roster requires distinct actor identities and valid decision history")
		}
		seen[id] = true
		roster = append(roster, id)
	}
	sort.Slice(roster, func(i, j int) bool {
		a, b := lastDecisionSequence[roster[i]], lastDecisionSequence[roster[j]]
		if a != b {
			return a < b
		}
		return roster[i] < roster[j]
	})
	if len(roster) > RPHotInitiativeLimit {
		roster = roster[:RPHotInitiativeLimit]
	}
	return roster, nil
}
