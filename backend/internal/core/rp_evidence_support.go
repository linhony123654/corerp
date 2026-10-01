package core

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const RPEvidenceSupportVersion = "corerp.evidence-support.v1"

// RPEvidenceSupportRange describes only a field in the supplied authorized view.
// It does not validate entailment of arbitrary proposal text, create canon, or
// grant visibility. Locators are JSON pointers relative to character; omitted
// times/targets remain unknown. Source handles and locators are packet-local.
type RPEvidenceSupportRange struct {
	Locator        string   `json:"locator"`
	Kind           string   `json:"kind"`
	ActorEntityID  string   `json:"actor_entity_id,omitempty"`
	TargetEntityID string   `json:"target_entity_id,omitempty"`
	WorldTime      string   `json:"world_time,omitempty"`
	Completeness   string   `json:"completeness"`
	Status         string   `json:"status,omitempty"`
	AllowedUses    []string `json:"allowed_uses"`
}

// RPDecisionEvidenceSupport compiles metadata after selection and identity
// projection, without fetching Events, interpreting text, or retaining values.
// Unknown domains and ancestral references deliberately remain provenance only.
func RPDecisionEvidenceSupport(in RPDecisionInput) map[string][]RPEvidenceSupportRange {
	out := map[string][]RPEvidenceSupportRange{}
	covered := map[string]bool{}
	add := func(id, sourceLocator, locator, kind, actor, target, at, completeness, status string, uses ...string) {
		if id == "" {
			return
		}
		covered[sourceLocator] = true
		out[id] = append(out[id], RPEvidenceSupportRange{locator, kind, actor, target, at, completeness, status, uses})
	}
	speech := func(id, source, locator, actor, at, completeness string) {
		uses := []string{"attributed_speech"}
		if completeness == "complete" {
			uses = append(uses, "exact_quote")
		}
		add(id, source, locator, "accepted_speech", actor, "", at, completeness, "said", uses...)
	}
	complete := func(text string, from bool) string {
		if text != "" {
			return "complete"
		}
		if from {
			return "reference_only"
		}
		return "missing"
	}
	if in.Persona != "" {
		add(in.PersonaSourceEventID, "/persona_source_event_id", "/persona", "authored_persona", in.NPCEntityID, "", "", "complete", "declared", "authored_characterization")
	}
	for i, r := range in.Relationships {
		base := fmt.Sprintf("/authored_relationships/%d", i)
		for _, field := range []struct {
			name    string
			present bool
			use     string
		}{{"role", r.Role != "", "declared_role"}, {"address_to", len(r.AddressTo) > 0, "declared_address"}, {"self_reference", r.SelfReference != "", "declared_self_reference"}} {
			if field.present {
				add(r.SourceEventID, base+"/source_event_id", base+"/"+field.name, "authored_relationship", in.NPCEntityID, r.SubjectEntityID, "", "complete", "declared", field.use)
			}
		}
	}
	speech(in.SpeechEventID, "/speech_event_id", "/player_speech_text", in.InterlocutorEntityID, in.PlayerSpeechWorldTime, complete(in.PlayerSpeechText, false))
	for i, d := range in.RecentDialogue {
		base := fmt.Sprintf("/recent_dialogue/%d", i)
		speech(d.EventID, base+"/event_id", base+"/text", d.SpeakerEntityID, d.WorldTime, complete(d.Text, d.TextFromEvent))
	}
	for i, e := range in.RelevantDialogue {
		for j, d := range e.Dialogue {
			base := fmt.Sprintf("/relevant_dialogue/%d/dialogue/%d", i, j)
			speech(d.EventID, base+"/event_id", base+"/text", d.SpeakerEntityID, d.WorldTime, complete(d.Text, d.TextFromEvent))
		}
	}
	for i, d := range in.HeardPlayerHistory {
		base := fmt.Sprintf("/heard_player_history/%d", i)
		c := complete(d.Excerpt, d.TextFromEvent)
		if d.Truncated {
			c = "partial"
		}
		speech(d.EventID, base+"/event_id", base+"/excerpt", in.InterlocutorEntityID, d.WorldTime, c)
	}
	for i, k := range in.Knowledge {
		base := fmt.Sprintf("/knowledge/%d", i)
		switch k.ClaimType {
		case "speaker_said":
			speech(k.SourceEventID, base+"/source_event_id", base+"/text", k.SubjectEntityID, "", complete(k.Text, k.TextFromEvent))
		case "agent_presence":
			if k.PlaceID != "" {
				add(k.SourceEventID, base+"/source_event_id", base+"/place_id", "observed_presence", k.SubjectEntityID, "", "", "complete", "observed", "sourced_presence")
			}
		case "nonverbal_action", "object_interaction":
			if k.Text != "" || k.TextFromEvent {
				add(k.SourceEventID, base+"/source_event_id", base+"/text", "observed_description", k.SubjectEntityID, "", "", complete(k.Text, k.TextFromEvent), "observed", "recorded_observation")
			}
		}
	}
	for i, m := range in.RecentPrivateDecisions {
		base := fmt.Sprintf("/recent_private_decisions/%d", i)
		add(m.SourceEventID, base+"/source_event_id", base+"/private", "own_private", in.NPCEntityID, m.InterlocutorEntityID, m.WorldTime, "complete", "historical_sketch", "private_continuity")
	}
	for i, a := range in.OwnActions {
		base := fmt.Sprintf("/own_actions/%d", i)
		if a.Action == "speech" {
			speech(a.EventID, base+"/event_id", base+"/text", in.NPCEntityID, a.WorldTime, complete(a.Text, a.TextFromEvent))
			continue
		}
		// Classify only the current server projection's recognized observable kinds.
		if a.Action == "leave" || a.Action == "activity" || a.Action == "nonverbal" || a.ActivityCode == "rp_object" {
			add(a.EventID, base+"/event_id", base, "own_observable", in.NPCEntityID, "", a.WorldTime, "complete", a.Status, "recorded_action_at_time")
		}
	}
	for i, a := range in.SceneActivities {
		if a.ObservationBasis == "current_visibility" || a.ObservationBasis == "own_action" {
			base := fmt.Sprintf("/scene_activities/%d", i)
			add(a.SourceEventID, base+"/source_event_id", base, "observed_activity", a.ActorID, "", a.WorldTime, "complete", a.Status, "activity_status_at_time")
		}
	}
	if s := in.NextSchedule; s != nil {
		add(s.SourceEventID, "/next_schedule/source_event_id", "/next_schedule", "schedule", in.NPCEntityID, "", s.WorldTime, "complete", "planned", "scheduled_plan")
	}
	// Reflect only to locate references, never to infer their evidentiary meaning.
	var walk func(reflect.Value, string)
	walk = func(v reflect.Value, path string) {
		for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		if !v.IsValid() {
			return
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				f := v.Type().Field(i)
				if !f.IsExported() {
					continue
				}
				name := strings.Split(f.Tag.Get("json"), ",")[0]
				if name == "-" {
					continue
				}
				if name == "" {
					name = f.Name
				}
				p := path + "/" + name
				child := v.Field(i)
				if strings.HasSuffix(f.Name, "EventID") && child.Kind() == reflect.String {
					if !covered[p] {
						add(child.String(), p, p, "provenance_only", "", "", "", "reference_only", "", "provenance")
					}
					continue
				}
				if strings.HasSuffix(f.Name, "EventIDs") && child.Kind() == reflect.Slice {
					for j := 0; j < child.Len(); j++ {
						q := fmt.Sprintf("%s/%d", p, j)
						add(child.Index(j).String(), q, q, "provenance_only", "", "", "", "reference_only", "", "provenance")
					}
					continue
				}
				walk(child, p)
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i), fmt.Sprintf("%s/%d", path, i))
			}
		}
	}
	walk(reflect.ValueOf(in), "")
	for id, ranges := range out {
		sort.Slice(ranges, func(i, j int) bool { return ranges[i].Locator < ranges[j].Locator })
		unique := ranges[:0]
		for _, r := range ranges {
			if len(unique) == 0 || !reflect.DeepEqual(unique[len(unique)-1], r) {
				unique = append(unique, r)
			}
		}
		out[id] = unique
	}
	return out
}
