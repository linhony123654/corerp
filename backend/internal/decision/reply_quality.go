package decision

import (
	"strings"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

// Quality hints never license a world effect. Semantic repetition is diagnostic
// only: repeating a promise when asked to recall it can be exactly right.
// Only the narrow wire-format hint offers the existing bounded rewrite.
func npcReplyQualityFeedback(input core.RPDecisionInput, proposal core.RPDecisionProposal) (string, string) {
	if proposal.Action != "respond" && proposal.Action != "refuse" {
		return "", ""
	}
	if tail := trailingNPCWireFragment(proposal.Text); tail != "" && !strings.Contains(input.PlayerSpeechText, tail) {
		return "speech_format_residue", "The previous proposal's text may contain trailing structured-output fragments after the spoken sentence. Regenerate the complete schema object from the same authorized context. Keep text to the character's intended spoken words, with schema keys and delimiters outside that string. Preserve any literal code or punctuation the conversation actually calls for. Do not add a world fact or an extra action."
	}
	if repeatsAcceptedNPCReply(input, proposal) {
		return "repetition", ""
	}
	return "", ""
}

// Look only for an ASCII tail beginning with excess closing structure after a
// finished sentence. Balanced literal data and in-world quotations are not
// removed or rejected. This intentionally narrow heuristic can still miss
// residue or flag intentional punctuation, so its result is only a hint.
func trailingNPCWireFragment(text string) string {
	boundary := strings.LastIndexAny(text, "。！？!?")
	if boundary < 0 {
		boundary = strings.LastIndex(text, ". ")
	}
	if boundary < 0 {
		return ""
	}
	_, size := utf8.DecodeRuneInString(text[boundary:])
	tail := strings.TrimSpace(text[boundary+size:])
	if tail == "" || (tail[0] != '}' && tail[0] != ']') {
		return ""
	}
	for _, char := range tail {
		if char < ' ' || char > '~' {
			return ""
		}
	}
	if strings.Count(tail, "}")+strings.Count(tail, "]")+strings.Count(tail, `"`) < 2 {
		return ""
	}
	// A closing brace in an ordinary literal JSON object is not excess.
	prefix := text[:boundary]
	if strings.Count(prefix, "{") > strings.Count(prefix, "}") || strings.Count(prefix, "[") > strings.Count(prefix, "]") {
		return ""
	}
	return tail
}
