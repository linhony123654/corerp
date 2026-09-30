package decision

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"corerp.local/backend/internal/core"
)

// Handles are bound to one authorized packet, not persistent RP state. The
// model chooses a short ref; existing world validation still receives Event IDs.
type decisionSourceRef struct {
	Ref           string `json:"ref"`
	SourceEventID string `json:"source_event_id"`
}

func decisionSourceRefs(input core.RPDecisionInput) []decisionSourceRef {
	allowed := core.RPDecisionEvidenceEventIDs(input)
	ids := make([]string, 0, len(allowed))
	for id := range allowed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	refs := make([]decisionSourceRef, 0, len(ids))
	next := 1
	for _, id := range ids {
		var ref string
		for {
			ref = fmt.Sprintf("src_%d", next)
			next++
			if !allowed[ref] {
				break
			}
		}
		refs = append(refs, decisionSourceRef{Ref: ref, SourceEventID: id})
	}
	return refs
}

func resolveDecisionSourceRefs(input core.RPDecisionInput, proposal core.RPDecisionProposal) core.RPDecisionProposal {
	if proposal.Private == nil {
		return proposal
	}
	bindings := make(map[string]string)
	for _, source := range decisionSourceRefs(input) {
		bindings[source.Ref] = source.SourceEventID
	}
	private := *proposal.Private
	private.BasisEventIDs = append([]string{}, private.BasisEventIDs...)
	for index, ref := range private.BasisEventIDs {
		if id, known := bindings[ref]; known {
			private.BasisEventIDs[index] = id
		}
		// Authorized raw IDs remain compatible with older v3 replies. Unknown
		// strings stay invalid: no approximation, removal or deduplication.
	}
	proposal.Private = &private
	return proposal
}

// Diagnose shape, never values, even when a response contains private text.
func debugDecisionProposalShape(raw string) {
	if os.Getenv("CORERP_DECISION_DEBUG") == "" {
		return
	}
	var root map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &root) != nil || root == nil {
		debugDecisionFailure("proposal schema shape", "json_object=false")
		return
	}
	var private map[string]json.RawMessage
	privateObject := json.Unmarshal(root["private"], &private) == nil && private != nil
	missing := 0
	for _, field := range []string{"intent", "emotion", "relationship_stance", "basis_event_ids"} {
		if private[field] == nil {
			missing++
		}
	}
	textTypes := true
	for _, field := range []string{"intent", "emotion", "relationship_stance"} {
		var value string
		textTypes = textTypes && len(private[field]) > 0 && private[field][0] == '"' && json.Unmarshal(private[field], &value) == nil
	}
	basisArray := len(private["basis_event_ids"]) > 0 && private["basis_event_ids"][0] == '['
	debugDecisionFailure("proposal schema shape", "json_object=true root_fields=", len(root), " private_object=", privateObject, " private_fields=", len(private), " private_missing=", missing, " private_text_types=", textTypes, " basis_array=", basisArray, " observable_present=", root["observable"] != nil)
}
