package decision

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Core classifies evidence with v1; v2 is only the lossless chat wire encoding.
const decisionEvidenceSupportWireVersion = "corerp.evidence-support.v2"

type decisionWireSourceRef struct {
	Ref             string                     `json:"ref"`
	SourceEventID   string                     `json:"source_event_id"`
	SupportDefaults map[string]json.RawMessage `json:"support_defaults,omitempty"`
	SupportRanges   []json.RawMessage          `json:"support_ranges"`
}

// compactDecisionSources shares only fields present and byte-identical in all
// ranges of one source. It never supplies unknown data, shares locators, merges
// sources/ranges, or interprets content. Single ranges retain their full object.
func compactDecisionSources(sources []decisionSourceRef) ([]decisionWireSourceRef, error) {
	out := make([]decisionWireSourceRef, 0, len(sources))
	for _, source := range sources {
		wire := decisionWireSourceRef{Ref: source.Ref, SourceEventID: source.SourceEventID, SupportRanges: make([]json.RawMessage, 0, len(source.SupportRanges))}
		fields := make([]map[string]json.RawMessage, len(source.SupportRanges))
		for i, r := range source.SupportRanges {
			raw, err := json.Marshal(r)
			if err != nil {
				return nil, err
			}
			wire.SupportRanges = append(wire.SupportRanges, raw)
			if len(source.SupportRanges) > 1 {
				if err = json.Unmarshal(raw, &fields[i]); err != nil {
					return nil, err
				}
			}
		}
		if len(fields) > 1 {
			for _, key := range []string{"actor_entity_id", "target_entity_id", "world_time", "kind", "completeness", "status", "allowed_uses"} {
				value, present := fields[0][key]
				if !present {
					continue
				}
				common := true
				for _, r := range fields[1:] {
					other, exists := r[key]
					if !exists || !bytes.Equal(value, other) {
						common = false
						break
					}
				}
				if !common {
					continue
				}
				if wire.SupportDefaults == nil {
					wire.SupportDefaults = map[string]json.RawMessage{}
				}
				wire.SupportDefaults[key] = value
				for _, r := range fields {
					delete(r, key)
				}
			}
			for i, r := range fields {
				raw, err := json.Marshal(r)
				if err != nil {
					return nil, err
				}
				wire.SupportRanges[i] = raw
			}
		}
		out = append(out, wire)
	}
	return out, nil
}

// Dictionary encoding is a second lossless representation for source-rich
// packets. Interning compares exact serialized field maps; omission is retained.
// It encodes original server ranges, not decoded model or test data.
func dictionaryDecisionSources(sources []decisionSourceRef) ([]decisionWireSourceRef, map[string]map[string]json.RawMessage, map[string]map[string]json.RawMessage, error) {
	profiles := map[string]map[string]json.RawMessage{}
	scopes := map[string]map[string]json.RawMessage{}
	profileIDs := map[string]string{}
	scopeIDs := map[string]string{}
	intern := func(fields map[string]json.RawMessage, keys []string, prefix string, ids map[string]string, table map[string]map[string]json.RawMessage) (json.RawMessage, error) {
		entry := map[string]json.RawMessage{}
		for _, key := range keys {
			if value, ok := fields[key]; ok {
				entry[key] = value
				delete(fields, key)
			}
		}
		if len(entry) == 0 {
			return nil, nil
		}
		raw, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		id, exists := ids[string(raw)]
		if !exists {
			id = fmt.Sprintf("%s%d", prefix, len(ids)+1)
			ids[string(raw)] = id
			table[id] = entry
		}
		return json.Marshal(id)
	}
	out := make([]decisionWireSourceRef, 0, len(sources))
	for _, source := range sources {
		wire := decisionWireSourceRef{Ref: source.Ref, SourceEventID: source.SourceEventID, SupportRanges: make([]json.RawMessage, 0, len(source.SupportRanges))}
		fields := make([]map[string]json.RawMessage, len(source.SupportRanges))
		for i, r := range source.SupportRanges {
			raw, err := json.Marshal(r)
			if err != nil {
				return nil, nil, nil, err
			}
			if err = json.Unmarshal(raw, &fields[i]); err != nil {
				return nil, nil, nil, err
			}
			profile, err := intern(fields[i], []string{"kind", "completeness", "status", "allowed_uses"}, "p", profileIDs, profiles)
			if err != nil {
				return nil, nil, nil, err
			}
			if profile != nil {
				fields[i]["profile_ref"] = profile
			}
			scope, err := intern(fields[i], []string{"actor_entity_id", "target_entity_id", "world_time"}, "s", scopeIDs, scopes)
			if err != nil {
				return nil, nil, nil, err
			}
			if scope != nil {
				fields[i]["scope_ref"] = scope
			}
		}
		if len(fields) > 1 {
			for _, key := range []string{"profile_ref", "scope_ref"} {
				value, present := fields[0][key]
				if !present {
					continue
				}
				common := true
				for _, r := range fields[1:] {
					other, ok := r[key]
					if !ok || !bytes.Equal(value, other) {
						common = false
						break
					}
				}
				if common {
					if wire.SupportDefaults == nil {
						wire.SupportDefaults = map[string]json.RawMessage{}
					}
					wire.SupportDefaults[key] = value
					for _, r := range fields {
						delete(r, key)
					}
				}
			}
		}
		for _, r := range fields {
			raw, err := json.Marshal(r)
			if err != nil {
				return nil, nil, nil, err
			}
			wire.SupportRanges = append(wire.SupportRanges, raw)
		}
		out = append(out, wire)
	}
	return out, profiles, scopes, nil
}
