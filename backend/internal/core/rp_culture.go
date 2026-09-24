package core

import "strings"

// Culture is a versioned description, not permission to act or authority to
// impose another person's beliefs. Storage must verify the referenced Events.
type RPCulture struct {
	CultureID      string          `json:"culture_id"`
	VersionEventID string          `json:"version_event_id"`
	ScopeKind      string          `json:"scope_kind"`
	ScopeID        string          `json:"scope_id"`
	Values         []string        `json:"values"`
	Norms          []RPCultureNorm `json:"norms"`
	Customs        []string        `json:"customs"`
	Taboos         []string        `json:"taboos"`
	Rituals        []string        `json:"rituals"`
	StatusSymbols  []string        `json:"status_symbols"`
	GroupIdentity  string          `json:"group_identity"`
	SubcultureOf   []string        `json:"subculture_of"`
	Transmission   []string        `json:"transmission"`
	Conflict       []string        `json:"conflict"`
	Evolution      []string        `json:"evolution"`
}

type RPCultureNorm struct {
	NormID     string `json:"norm_id"`
	Action     string `json:"action"`
	Evaluation int    `json:"evaluation"` // -2..2, advisory social evaluation only
}

// A stance pins the version actually learned; newer unseen definitions must
// not silently replace it. Evidence authenticity belongs to the storage reader.
type RPCultureInternalization struct {
	KnowledgeKind       string `json:"knowledge_kind,omitempty"`
	EntityID            string `json:"entity_id"`
	CultureID           string `json:"culture_id"`
	VersionEventID      string `json:"version_event_id"`
	TransmissionEventID string `json:"transmission_event_id"`
	StanceEventID       string `json:"stance_event_id"`
	Stance              string `json:"stance"`
}

type RPCultureEvaluation struct {
	NormID string                   `json:"norm_id"`
	Score  int                      `json:"score"`
	Basis  RPCultureInternalization `json:"basis"`
}

// Private derived experience: the act is observed, but this evaluation belongs
// only to its evaluator. It must never be copied into participant-wide speech.
type RPCultureExperience struct {
	Conflicts     []RPCultureConflict   `json:"conflicts,omitempty"`
	ActionEventID string                `json:"action_event_id"`
	ActorEntityID string                `json:"actor_entity_id"`
	Evaluations   []RPCultureEvaluation `json:"evaluations"`
}

// Indices refer to the retained sourced evaluations, not hidden group beliefs.
type RPCultureConflict struct {
	First  int `json:"first"`
	Second int `json:"second"`
}

func RPCultureConflicts(evaluations []RPCultureEvaluation) []RPCultureConflict {
	var conflicts []RPCultureConflict
	for i, a := range evaluations {
		for j := i + 1; j < len(evaluations); j++ {
			b := evaluations[j]
			if (a.Score < 0 && b.Score > 0) || (a.Score > 0 && b.Score < 0) {
				conflicts = append(conflicts, RPCultureConflict{First: i, Second: j})
				if len(conflicts) == 8 {
					return conflicts
				}
			}
		}
	}
	return conflicts
}

// Membership is an explicit affiliation, not an endorsement of every norm.
type RPCultureAffiliation struct {
	PresenceEventID      string `json:"presence_event_id,omitempty"`
	ScopePlaceID         string `json:"scope_place_id,omitempty"`
	EligibilityEventID   string `json:"eligibility_event_id,omitempty"`
	EmploymentContractID string `json:"employment_contract_id,omitempty"`
	EntityID             string `json:"entity_id"`
	CultureID            string `json:"culture_id"`
	DefinitionEventID    string `json:"definition_event_id"`
	KnowledgeEventID     string `json:"knowledge_event_id"`
	SourceEventID        string `json:"source_event_id"`
	Status               string `json:"status"`
}

func (c RPCulture) Validate() error {
	for _, s := range []string{c.CultureID, c.VersionEventID, c.ScopeID, c.GroupIdentity} {
		if strings.TrimSpace(s) == "" || len(s) > 256 {
			return NewError(CodeInvalidArgument, "invalid culture identity or source")
		}
	}
	switch c.ScopeKind {
	case "world", "region", "community", "organization", "family":
	default:
		return NewError(CodeInvalidArgument, "unsupported culture scope")
	}
	for _, list := range [][]string{c.Values, c.Customs, c.Taboos, c.Rituals, c.StatusSymbols, c.SubcultureOf, c.Transmission, c.Conflict, c.Evolution} {
		if len(list) > 32 {
			return NewError(CodeInvalidArgument, "culture list exceeds bound")
		}
		for _, s := range list {
			if strings.TrimSpace(s) == "" || len(s) > 512 {
				return NewError(CodeInvalidArgument, "invalid culture description")
			}
		}
	}
	if len(c.Norms) == 0 || len(c.Norms) > 32 {
		return NewError(CodeInvalidArgument, "culture requires bounded norms")
	}
	seen := map[string]bool{}
	for _, n := range c.Norms {
		if strings.TrimSpace(n.NormID) == "" || len(n.NormID) > 256 || seen[n.NormID] || n.Action != "gift" || n.Evaluation < -2 || n.Evaluation > 2 {
			return NewError(CodeInvalidArgument, "invalid or unsupported culture norm")
		}
		seen[n.NormID] = true
	}
	return nil
}

// EvaluateRPCulture preserves each applicable norm independently. It does not
// rank executable actions, grant capabilities, change relationships or punish.
func EvaluateRPCulture(c RPCulture, own RPCultureInternalization, actorID, action string) ([]RPCultureEvaluation, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	for _, s := range []string{own.EntityID, own.CultureID, own.VersionEventID, own.StanceEventID, actorID} {
		if strings.TrimSpace(s) == "" || len(s) > 256 {
			return nil, NewError(CodeInvalidArgument, "culture evaluation requires own sourced knowledge")
		}
	}
	if own.KnowledgeKind == "authored" {
		if own.TransmissionEventID != "" {
			return nil, NewError(CodeInvalidArgument, "authored knowledge is not a fabricated transmission")
		}
	} else if own.KnowledgeKind != "" || strings.TrimSpace(own.TransmissionEventID) == "" || len(own.TransmissionEventID) > 256 {
		return nil, NewError(CodeInvalidArgument, "culture evaluation requires received or authored knowledge")
	}
	if own.EntityID != actorID || own.CultureID != c.CultureID || own.VersionEventID != c.VersionEventID {
		return nil, NewError(CodeInvalidArgument, "culture evaluation source does not match own known version")
	}
	switch own.Stance {
	case "accept", "partial", "oppose", "rebel":
	default:
		return nil, NewError(CodeInvalidArgument, "unsupported culture stance")
	}
	result := []RPCultureEvaluation{}
	for _, n := range c.Norms {
		if n.Action != action {
			continue
		}
		score := n.Evaluation
		switch own.Stance {
		case "partial":
			score /= 2
		case "oppose":
			score = 0
		case "rebel":
			score = -score
		}
		result = append(result, RPCultureEvaluation{NormID: n.NormID, Score: score, Basis: own})
	}
	return result, nil
}
