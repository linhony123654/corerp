package core

// RPSpeechTone is an actor-chosen, observable delivery of an actual utterance.
// It is neither the actor's private emotion nor a change in hearing range.
// Empty means no delivery was recorded, rather than a neutral inferred tone.
func ValidRPSpeechTone(tone string) bool {
	switch tone {
	case "", "gentle", "firm", "teasing", "hesitant", "flat":
		return true
	default:
		return false
	}
}
