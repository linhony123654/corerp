package core

import (
	"encoding/json"
	"sort"
	"strings"
)

const DefaultRPDecisionContextBudgetBytes = 64 << 10

const rpContextSelectionVersion = "corerp.context-selection.v1"

// This is NPC-only selection metadata over authorized, bounded candidates.
// It is neither a world fact nor a claim that all history is in the packet.
type RPContextSelection struct {
	PolicyVersion  string             `json:"policy_version"`
	Scope          string             `json:"scope"`
	BudgetBytes    int                `json:"budget_bytes"`
	EncodedBytes   int                `json:"encoded_bytes"`
	TextReferences int                `json:"text_references"`
	Omitted        RPContextOmissions `json:"omitted"`
}

type RPContextOmissions struct {
	RecentDialogue     int `json:"recent_dialogue,omitempty"`
	RelevantExchanges  int `json:"relevant_exchanges,omitempty"`
	HeardPlayerHistory int `json:"heard_player_history,omitempty"`
	OwnActions         int `json:"own_actions,omitempty"`
	PrivateDecisions   int `json:"private_decisions,omitempty"`
	Knowledge          int `json:"knowledge,omitempty"`
	SalientSpeech      int `json:"salient_speech,omitempty"`
}

// SelectRPDecisionContext finalizes the existing packet, not a new data source.
func SelectRPDecisionContext(input RPDecisionInput, budget int) (RPDecisionInput, error) {
	if budget <= 0 || budget > 128<<10 {
		return RPDecisionInput{}, NewError(CodeInvalidArgument, "RP context selection requires a bounded byte budget")
	}
	quotes, err := rpContextQuotes(input)
	if err != nil {
		return RPDecisionInput{}, err
	}
	base := input
	base.RecentDialogue, base.RelevantDialogue, base.HeardPlayerHistory = nil, nil, nil
	base.RecentPrivateDecisions, base.OwnActions, base.Knowledge = nil, nil, nil
	if input.Life != nil {
		life := *input.Life
		life.SalientMemories = nil
		for _, memory := range input.Life.SalientMemories {
			if memory.Kind != "speaker_said" {
				life.SalientMemories = append(life.SalientMemories, memory)
			}
		}
		base.Life = &life
	}
	previous := RPContextOmissions{}
	if input.ContextSelection != nil {
		previous = input.ContextSelection.Omitted
	}
	base.ContextSelection = &RPContextSelection{PolicyVersion: rpContextSelectionVersion, Scope: "bounded_authorized_candidates", BudgetBytes: budget}
	type candidate struct{ kind, index, priority, order int }
	var candidates []candidate
	// Only the recent local exchange gets preference over topic-selected older
	// exchanges. Other speakers cannot crowd all of the actor's own motivation out.
	peer := 0
	for i := len(input.RecentDialogue) - 1; i >= 0; i-- {
		d := input.RecentDialogue[i]
		priority := 5
		if peer < 4 && (d.SpeakerEntityID == input.NPCEntityID || d.SpeakerEntityID == input.InterlocutorEntityID) {
			priority = 1
			peer++
		}
		candidates = append(candidates, candidate{0, i, priority, -i})
	}
	for i := range input.RelevantDialogue {
		candidates = append(candidates, candidate{1, i, 3, -i})
	}
	for i := range input.RecentPrivateDecisions {
		candidates = append(candidates, candidate{2, i, 4, -i})
	}
	for i := range input.HeardPlayerHistory {
		candidates = append(candidates, candidate{3, i, 6, -i})
	}
	beats := 0
	for i, action := range input.OwnActions {
		priority := 7
		if action.Action != "speech" && beats < 2 {
			priority = 2
			beats++
		}
		candidates = append(candidates, candidate{4, i, priority, i})
	}
	for i, claim := range input.Knowledge {
		if claim.ClaimType == "speaker_said" {
			candidates = append(candidates, candidate{5, i, 5, i})
		} else {
			base.Knowledge = append(base.Knowledge, claim)
		}
	}
	if input.Life != nil {
		for i, memory := range input.Life.SalientMemories {
			if memory.Kind == "speaker_said" {
				candidates = append(candidates, candidate{6, i, 5, i})
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.priority != b.priority {
			return a.priority < b.priority
		}
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return a.order < b.order
	})
	selected := make([]bool, len(candidates))
	assemble := func() RPDecisionInput {
		out := base
		// Reconstruct in original event order, never priority order. Selection
		// priority cannot rewrite chronology or an exchange's attribution.
		keep := [7]map[int]bool{}
		for i := range keep {
			keep[i] = map[int]bool{}
		}
		for i, c := range candidates {
			if selected[i] {
				keep[c.kind][c.index] = true
			}
		}
		for i, d := range input.RecentDialogue {
			if keep[0][i] {
				out.RecentDialogue = append(out.RecentDialogue, d)
			}
		}
		for i, d := range input.RelevantDialogue {
			if keep[1][i] {
				out.RelevantDialogue = append(out.RelevantDialogue, d)
			}
		}
		for i, d := range input.RecentPrivateDecisions {
			if keep[2][i] {
				out.RecentPrivateDecisions = append(out.RecentPrivateDecisions, d)
			}
		}
		for i, d := range input.HeardPlayerHistory {
			if keep[3][i] {
				out.HeardPlayerHistory = append(out.HeardPlayerHistory, d)
			}
		}
		for i, d := range input.OwnActions {
			if keep[4][i] {
				out.OwnActions = append(out.OwnActions, d)
			}
		}
		out.Knowledge = nil
		for i, d := range input.Knowledge {
			if d.ClaimType != "speaker_said" || keep[5][i] {
				out.Knowledge = append(out.Knowledge, d)
			}
		}
		omittedSpeech := 0
		if input.Life != nil {
			life := *input.Life
			life.SalientMemories = nil
			for i, memory := range input.Life.SalientMemories {
				if memory.Kind != "speaker_said" || keep[6][i] {
					life.SalientMemories = append(life.SalientMemories, memory)
				} else {
					omittedSpeech++
				}
			}
			out.Life = &life
		}
		meta := *base.ContextSelection
		meta.Omitted = RPContextOmissions{
			RecentDialogue:     previous.RecentDialogue + len(input.RecentDialogue) - len(out.RecentDialogue),
			RelevantExchanges:  previous.RelevantExchanges + len(input.RelevantDialogue) - len(out.RelevantDialogue),
			PrivateDecisions:   previous.PrivateDecisions + len(input.RecentPrivateDecisions) - len(out.RecentPrivateDecisions),
			HeardPlayerHistory: previous.HeardPlayerHistory + len(input.HeardPlayerHistory) - len(out.HeardPlayerHistory),
			OwnActions:         previous.OwnActions + len(input.OwnActions) - len(out.OwnActions),
			Knowledge:          previous.Knowledge + len(input.Knowledge) - len(out.Knowledge),
			SalientSpeech:      previous.SalientSpeech + omittedSpeech,
		}
		out.ContextSelection = &meta
		return out
	}
	measure := func() (RPDecisionInput, int, error) {
		out, err := normalizeRPContextQuotes(assemble(), quotes)
		if err != nil {
			return RPDecisionInput{}, 0, err
		}
		// The byte count includes its own metadata. A short fixed-point loop
		// measures the actual JSON, including escaped quotes and UTF-8 bytes.
		for i := 0; i < 8; i++ {
			encoded, err := json.Marshal(out)
			if err != nil {
				return RPDecisionInput{}, 0, err
			}
			if out.ContextSelection.EncodedBytes == len(encoded) {
				return out, len(encoded), nil
			}
			out.ContextSelection.EncodedBytes = len(encoded)
		}
		return RPDecisionInput{}, 0, NewError(CodeProjectionDiverged, "RP context byte count did not stabilize")
	}
	result, size, err := measure()
	if err != nil {
		return RPDecisionInput{}, err
	}
	if size > budget {
		return RPDecisionInput{}, NewError(CodeInvalidArgument, "required RP context exceeds selection budget")
	}
	for i := range candidates {
		selected[i] = true
		trial, size, err := measure()
		if err != nil {
			return RPDecisionInput{}, err
		}
		if size > budget {
			selected[i] = false
		} else {
			result = trial
		}
	}
	return result, nil
}

type rpContextQuoteKey struct{ event, speaker string }

type rpContextQuote struct{ speaker, text string }

// ResolveRPDecisionSpeech returns only complete, attributed observable words
// actually in this packet. Private sketches and partial excerpts are excluded.
func ResolveRPDecisionSpeech(input RPDecisionInput, eventID, speakerID string) (string, bool) {
	quotes, err := rpContextQuotes(input)
	if err != nil {
		return "", false
	}
	quote, found := quotes[rpContextQuoteKey{eventID, speakerID}]
	return quote.text, found
}

// Private intent is deliberately absent from this index: equal wording is not
// grounds for converting an internal decision into an utterance.
func rpContextQuotes(in RPDecisionInput) (map[rpContextQuoteKey]rpContextQuote, error) {
	quotes := map[rpContextQuoteKey]rpContextQuote{}
	// Actual accepted-utterance views identify one speaker per Event. A
	// declarative knowledge source can support several distinct statements;
	// those cannot be made into one reference merely by sharing provenance.
	canonicalActors := map[string]string{}
	canonical := func(id, speaker string) error {
		if id == "" || speaker == "" {
			return nil
		}
		if old, ok := canonicalActors[id]; ok && old != speaker {
			return NewError(CodeProjectionDiverged, "accepted RP utterance has conflicting speakers")
		}
		canonicalActors[id] = speaker
		return nil
	}
	if err := canonical(in.SpeechEventID, in.InterlocutorEntityID); err != nil {
		return nil, err
	}
	for _, d := range in.RecentDialogue {
		if err := canonical(d.EventID, d.SpeakerEntityID); err != nil {
			return nil, err
		}
	}
	for _, g := range in.RelevantDialogue {
		for _, d := range g.Dialogue {
			if err := canonical(d.EventID, d.SpeakerEntityID); err != nil {
				return nil, err
			}
		}
	}
	for _, d := range in.OwnActions {
		if d.Action == "speech" {
			if err := canonical(d.EventID, in.NPCEntityID); err != nil {
				return nil, err
			}
		}
	}
	for _, d := range in.HeardPlayerHistory {
		if err := canonical(d.EventID, in.InterlocutorEntityID); err != nil {
			return nil, err
		}
	}
	ambiguous := map[rpContextQuoteKey]bool{}
	add := func(id, speaker, text string, ref bool) error {
		if ref && text != "" {
			return NewError(CodeProjectionDiverged, "RP speech reference also contains a body")
		}
		if id == "" || speaker == "" || text == "" {
			return nil
		}
		if actor, ok := canonicalActors[id]; ok && actor != speaker {
			return NewError(CodeProjectionDiverged, "knowledge conflicts with accepted RP speaker")
		}
		key := rpContextQuoteKey{id, speaker}
		if ambiguous[key] {
			return nil
		}
		if old, ok := quotes[key]; ok && old.text != text {
			if _, canonical := canonicalActors[id]; canonical {
				return NewError(CodeProjectionDiverged, "RP speech source has conflicting attribution or words")
			}
			delete(quotes, key)
			ambiguous[key] = true
			return nil
		}
		quotes[rpContextQuoteKey{id, speaker}] = rpContextQuote{speaker, text}
		return nil
	}
	if err := add(in.SpeechEventID, in.InterlocutorEntityID, in.PlayerSpeechText, false); err != nil {
		return nil, err
	}
	for _, d := range in.RecentDialogue {
		if err := add(d.EventID, d.SpeakerEntityID, d.Text, d.TextFromEvent); err != nil {
			return nil, err
		}
	}
	for _, g := range in.RelevantDialogue {
		for _, d := range g.Dialogue {
			if err := add(d.EventID, d.SpeakerEntityID, d.Text, d.TextFromEvent); err != nil {
				return nil, err
			}
		}
	}
	for _, d := range in.Knowledge {
		if d.ClaimType == "speaker_said" {
			if err := add(d.SourceEventID, d.SubjectEntityID, d.Text, d.TextFromEvent); err != nil {
				return nil, err
			}
		}
	}
	if in.Life != nil {
		for _, d := range in.Life.SalientMemories {
			if d.Kind == "speaker_said" {
				if err := add(d.SourceEventID, d.SubjectEntityID, d.Text, d.TextFromEvent); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, d := range in.OwnActions {
		if d.Action == "speech" {
			if err := add(d.EventID, in.NPCEntityID, d.Text, d.TextFromEvent); err != nil {
				return nil, err
			}
		}
	}
	for _, d := range in.HeardPlayerHistory {
		if !d.Truncated {
			if err := add(d.EventID, in.InterlocutorEntityID, d.Excerpt, d.TextFromEvent); err != nil {
				return nil, err
			}
		}
	}
	check := func(id, speaker string, ref bool) error {
		if ref {
			q, ok := quotes[rpContextQuoteKey{id, speaker}]
			if !ok || q.speaker != speaker {
				return NewError(CodeProjectionDiverged, "RP speech reference lacks its authorized complete utterance")
			}
		}
		return nil
	}
	for _, d := range in.RecentDialogue {
		if err := check(d.EventID, d.SpeakerEntityID, d.TextFromEvent); err != nil {
			return nil, err
		}
	}
	for _, g := range in.RelevantDialogue {
		for _, d := range g.Dialogue {
			if err := check(d.EventID, d.SpeakerEntityID, d.TextFromEvent); err != nil {
				return nil, err
			}
		}
	}
	for _, d := range in.Knowledge {
		if d.TextFromEvent && d.ClaimType != "speaker_said" {
			return nil, NewError(CodeProjectionDiverged, "non-speech knowledge cannot refer to utterance text")
		}
		if err := check(d.SourceEventID, d.SubjectEntityID, d.TextFromEvent); err != nil {
			return nil, err
		}
	}
	if in.Life != nil {
		for _, d := range in.Life.SalientMemories {
			if d.TextFromEvent && d.Kind != "speaker_said" {
				return nil, NewError(CodeProjectionDiverged, "non-speech life memory cannot refer to utterance text")
			}
			if err := check(d.SourceEventID, d.SubjectEntityID, d.TextFromEvent); err != nil {
				return nil, err
			}
		}
	}
	for _, d := range in.OwnActions {
		if d.TextFromEvent && d.Action != "speech" {
			return nil, NewError(CodeProjectionDiverged, "non-speech action cannot refer to utterance text")
		}
		if err := check(d.EventID, in.NPCEntityID, d.TextFromEvent); err != nil {
			return nil, err
		}
	}
	for _, d := range in.HeardPlayerHistory {
		if err := check(d.EventID, in.InterlocutorEntityID, d.TextFromEvent); err != nil {
			return nil, err
		}
	}
	return quotes, nil
}

func normalizeRPContextQuotes(in RPDecisionInput, quotes map[rpContextQuoteKey]rpContextQuote) (RPDecisionInput, error) {
	out := in
	out.RecentDialogue = append([]RPDecisionDialogue(nil), in.RecentDialogue...)
	out.RelevantDialogue = make([]RPDecisionExchange, len(in.RelevantDialogue))
	for i, g := range in.RelevantDialogue {
		out.RelevantDialogue[i].Dialogue = append([]RPDecisionDialogue(nil), g.Dialogue...)
	}
	out.Knowledge = append([]RPDecisionKnowledge(nil), in.Knowledge...)
	out.OwnActions = append([]RPOwnAction(nil), in.OwnActions...)
	out.HeardPlayerHistory = append([]RPDecisionSpeechExcerpt(nil), in.HeardPlayerHistory...)
	if in.Life != nil {
		life := *in.Life
		life.SalientMemories = append([]RPLifeMemory(nil), in.Life.SalientMemories...)
		out.Life = &life
	}
	seen := map[rpContextQuoteKey]bool{}
	if in.SpeechEventID != "" && in.PlayerSpeechText != "" {
		seen[rpContextQuoteKey{in.SpeechEventID, in.InterlocutorEntityID}] = true
	}
	references := 0
	rewrite := func(id, speaker string, text *string, ref *bool) error {
		if id == "" || speaker == "" {
			return nil
		}
		q, ok := quotes[rpContextQuoteKey{id, speaker}]
		if !ok {
			return nil
		}
		if q.speaker != speaker {
			return NewError(CodeProjectionDiverged, "RP utterance attribution changed")
		}
		if seen[rpContextQuoteKey{id, speaker}] {
			*text = ""
			*ref = true
			references++
		} else {
			*text = q.text
			*ref = false
			seen[rpContextQuoteKey{id, speaker}] = true
		}
		return nil
	}
	for i := range out.RecentDialogue {
		d := &out.RecentDialogue[i]
		if err := rewrite(d.EventID, d.SpeakerEntityID, &d.Text, &d.TextFromEvent); err != nil {
			return RPDecisionInput{}, err
		}
	}
	for i := range out.RelevantDialogue {
		for j := range out.RelevantDialogue[i].Dialogue {
			d := &out.RelevantDialogue[i].Dialogue[j]
			if err := rewrite(d.EventID, d.SpeakerEntityID, &d.Text, &d.TextFromEvent); err != nil {
				return RPDecisionInput{}, err
			}
		}
	}
	for i := range out.Knowledge {
		d := &out.Knowledge[i]
		if d.ClaimType == "speaker_said" {
			if err := rewrite(d.SourceEventID, d.SubjectEntityID, &d.Text, &d.TextFromEvent); err != nil {
				return RPDecisionInput{}, err
			}
		}
	}
	if out.Life != nil {
		for i := range out.Life.SalientMemories {
			d := &out.Life.SalientMemories[i]
			if d.Kind == "speaker_said" {
				if err := rewrite(d.SourceEventID, d.SubjectEntityID, &d.Text, &d.TextFromEvent); err != nil {
					return RPDecisionInput{}, err
				}
			}
		}
	}
	for i := range out.OwnActions {
		d := &out.OwnActions[i]
		if d.Action == "speech" {
			if err := rewrite(d.EventID, in.NPCEntityID, &d.Text, &d.TextFromEvent); err != nil {
				return RPDecisionInput{}, err
			}
		}
	}
	for i := range out.HeardPlayerHistory {
		d := &out.HeardPlayerHistory[i]
		key := rpContextQuoteKey{d.EventID, in.InterlocutorEntityID}
		if q, ok := quotes[key]; ok {
			if d.Truncated && d.Excerpt != "" && !strings.HasPrefix(q.text, strings.TrimSuffix(d.Excerpt, "…")) {
				return RPDecisionInput{}, NewError(CodeProjectionDiverged, "RP speech excerpt conflicts with complete utterance")
			}
			// If the complete statement could not be selected, keep this
			// explicitly partial excerpt; do not inflate it into a full quote.
			if d.Truncated && !d.TextFromEvent && !seen[key] {
				continue
			}
			if err := rewrite(d.EventID, in.InterlocutorEntityID, &d.Excerpt, &d.TextFromEvent); err != nil {
				return RPDecisionInput{}, err
			}
		}
	}
	meta := *in.ContextSelection
	meta.TextReferences = references
	out.ContextSelection = &meta
	return out, nil
}
