package decision

import (
	"strings"
	"unicode"

	"corerp.local/backend/internal/core"
)

// This heuristic does not decide whether a reply is semantically correct.
// Its result is diagnostic only and does not trigger a model rewrite or block
// an otherwise valid proposal. Observable approval still belongs to the owner.
func repeatsAcceptedNPCReply(input core.RPDecisionInput, proposal core.RPDecisionProposal) bool {
	if proposal.Action != "respond" && proposal.Action != "refuse" {
		return false
	}
	checked := 0
	recent := make([]string, 0, 6)
	for i := len(input.RecentDialogue) - 1; i >= 0 && checked < 6; i-- {
		prior := input.RecentDialogue[i]
		if prior.SpeakerEntityID != input.NPCEntityID {
			continue
		}
		checked++
		recent = append(recent, prior.Text)
		if repliesAreRepetitive(proposal.Text, prior.Text) {
			return true
		}
	}
	return repeatsCommonNPCFragment(proposal.Text, input.PlayerSpeechText, recent)
}

// Repeated stock advice can hide inside otherwise distinct answers. A phrase
// shared with at least two of this NPC's earlier accepted replies is flagged
// unless it came from the player's current wording.
func repeatsCommonNPCFragment(reply, player string, recent []string) bool {
	const size = 5
	candidate, playerWords := normalizedReply(reply), string(normalizedReply(player))
	if len(candidate) < 12 || len(recent) < 2 {
		return false
	}
	counts := map[string]int{}
	for _, prior := range recent {
		words, once := normalizedReply(prior), map[string]bool{}
		for i := 0; i+size <= len(words); i++ {
			fragment := string(words[i : i+size])
			if !once[fragment] {
				once[fragment] = true
				counts[fragment]++
			}
		}
	}
	for i := 0; i+size <= len(candidate); i++ {
		fragment := string(candidate[i : i+size])
		if counts[fragment] >= 2 && !strings.Contains(playerWords, fragment) {
			return true
		}
	}
	return false
}

func repliesAreRepetitive(left, right string) bool {
	a, b := normalizedReply(left), normalizedReply(right)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if string(a) == string(b) {
		return true
	}
	if len(a) < 5 || len(b) < 5 {
		return false
	}
	gramsA, gramsB := replyBigrams(a), replyBigrams(b)
	shared := 0
	for gram := range gramsA {
		if gramsB[gram] {
			shared++
		}
	}
	overlap := float64(2*shared) / float64(len(gramsA)+len(gramsB))
	if overlap >= 0.72 {
		return true
	}
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	return prefix >= 5 && overlap >= 0.52
}

func normalizedReply(value string) []rune {
	result := make([]rune, 0, len(value))
	for _, character := range strings.ToLower(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			result = append(result, character)
		}
	}
	return result
}

func replyBigrams(value []rune) map[string]bool {
	grams := make(map[string]bool, len(value)-1)
	for i := 0; i+1 < len(value); i++ {
		grams[string(value[i:i+2])] = true
	}
	return grams
}
