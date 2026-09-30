package storage

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

const rpDialogueCandidateLimit = 512
const rpRelevantExchangeLimit = 4
const rpRelevantDialogueRuneBudget = 6000

type rpDialogueCandidate struct {
	dialogue core.RPDecisionDialogue
	sequence int64
	session  string
	turn     string
}

// Retrieval is a read projection over the same accepted/heard speech as the
// recent window. No summary, model call, unobserved turn content or new canon.
func readRPRelevantDialogue(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) ([]core.RPDecisionExchange, error) {
	if strings.TrimSpace(input.PlayerSpeechText) == "" {
		return nil, nil
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT u.speaker_entity_id,u.speech_text,u.event_id,u.world_time,e.event_sequence,
		       u.session_id,COALESCE(NULLIF(json_extract(e.payload,'$.parent_turn_id'),''),u.turn_id)
		FROM rp_utterances u JOIN events e ON e.event_id=u.event_id
		WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=?
		  AND (u.speaker_entity_id=? OR EXISTS (
		    SELECT 1 FROM observation_records o
		    WHERE o.source_event_id=u.event_id AND o.observer_agent_id=?
		      AND o.subject_agent_id=u.speaker_entity_id
		      AND o.claim_key='speech:' || u.event_id
		      AND json_extract(o.claim_payload,'$.claim_type')='speaker_said'))
		ORDER BY e.event_sequence DESC LIMIT ?`, input.InstanceID, input.BranchID, input.HeadSequence,
		input.NPCEntityID, input.NPCEntityID, rpDialogueCandidateLimit)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read NPC relevant dialogue candidates", err)
	}
	defer rows.Close()
	var candidates []rpDialogueCandidate
	for rows.Next() {
		var candidate rpDialogueCandidate
		d := &candidate.dialogue
		if err := rows.Scan(&d.SpeakerEntityID, &d.Text, &d.EventID, &d.WorldTime, &candidate.sequence, &candidate.session, &candidate.turn); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan NPC relevant dialogue", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate NPC relevant dialogue", err)
	}
	return selectRPRelevantDialogue(input, candidates), nil
}

// Rank shared words/Han bigrams using their frequency in this authorized
// candidate set. This is lexical relevance only: paraphrases may not match.
// Sort every ranking input/tie so a reread at the same head has the same hash.
func selectRPRelevantDialogue(input core.RPDecisionInput, candidates []rpDialogueCandidate) []core.RPDecisionExchange {
	type group struct {
		dialogue []rpDialogueCandidate
		terms    map[string]bool
		sequence int64
		recent   bool
		score    float64
	}
	recent := map[string]bool{}
	for _, d := range input.RecentDialogue {
		recent[d.EventID] = true
	}
	groups := map[[2]string]*group{}
	for _, c := range candidates {
		key := [2]string{c.session, c.turn}
		g := groups[key]
		if g == nil {
			g = &group{terms: map[string]bool{}}
			groups[key] = g
		}
		g.dialogue = append(g.dialogue, c)
		if c.sequence > g.sequence {
			g.sequence = c.sequence
		}
		if recent[c.dialogue.EventID] || c.dialogue.EventID == input.SpeechEventID {
			g.recent = true
		}
		for term := range rpDialogueTerms(c.dialogue.Text) {
			g.terms[term] = true
		}
	}
	query := rpDialogueTerms(input.PlayerSpeechText)
	terms := make([]string, 0, len(query))
	for term := range query {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	frequency := map[string]int{}
	for _, g := range groups {
		for _, term := range terms {
			if g.terms[term] {
				frequency[term]++
			}
		}
	}
	var ranked []*group
	for _, g := range groups {
		if g.recent {
			continue
		}
		for _, term := range terms {
			if g.terms[term] {
				// Common conversational fragments should not displace a distinct topic.
				if len(groups) > 3 && frequency[term]*2 > len(groups) {
					continue
				}
				g.score += math.Log(1 + float64(len(groups))/float64(frequency[term]))
			}
		}
		if g.score > 0 {
			ranked = append(ranked, g)
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].sequence > ranked[j].sequence
	})
	var selected []*group
	remaining := rpRelevantDialogueRuneBudget
	for _, g := range ranked {
		size := 0
		for _, c := range g.dialogue {
			size += utf8.RuneCountInString(c.dialogue.Text)
		}
		if size > remaining {
			continue
		} // Preserve complete original utterances.
		remaining -= size
		selected = append(selected, g)
		if len(selected) == rpRelevantExchangeLimit {
			break
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].sequence < selected[j].sequence })
	var result []core.RPDecisionExchange
	for _, g := range selected {
		sort.Slice(g.dialogue, func(i, j int) bool { return g.dialogue[i].sequence < g.dialogue[j].sequence })
		exchange := core.RPDecisionExchange{}
		for _, c := range g.dialogue {
			exchange.Dialogue = append(exchange.Dialogue, c.dialogue)
		}
		result = append(result, exchange)
	}
	return result
}

func rpDialogueTerms(text string) map[string]bool {
	terms := map[string]bool{}
	var word []rune
	var previousHan rune
	flush := func() {
		if len(word) > 1 {
			terms[string(word)] = true
		}
		word = word[:0]
	}
	for _, r := range strings.ToLower(text) {
		if unicode.Is(unicode.Han, r) {
			flush()
			if previousHan != 0 {
				terms[string([]rune{previousHan, r})] = true
			}
			previousHan = r
		} else {
			previousHan = 0
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				word = append(word, r)
			} else {
				flush()
			}
		}
	}
	flush()
	return terms
}
