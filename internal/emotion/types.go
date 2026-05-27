package emotion

import "time"

// EmotionVector is a multi-dimensional emotional state.
// Unlike canonical state (deterministic, single-truth), emotions
// are allowed to be contradictory: attachment=0.8 + resentment=0.3
// is valid and realistic.
type EmotionVector struct {
	// Primary dimensions (Ekman + relationship-specific)
	Joy      float64 `json:"joy"`
	Sadness  float64 `json:"sadness"`
	Anger    float64 `json:"anger"`
	Fear     float64 `json:"fear"`
	Trust    float64 `json:"trust"`
	Disgust  float64 `json:"disgust"`
	Surprise float64 `json:"surprise"`

	// Relationship dimensions (contradiction-friendly)
	Attachment float64 `json:"attachment"` // wanting closeness
	Resentment float64 `json:"resentment"` // accumulated grievance
	Gratitude  float64 `json:"gratitude"`  // feeling of debt/thanks
	Guilt      float64 `json:"guilt"`      // self-blame
	Longing    float64 `json:"longing"`    // missing someone/something
}

// Dominant returns just the name of the strongest emotion.
func (ev EmotionVector) Dominant() string {
	name, _ := ev.DominantEmotion()
	return name
}

// DominantEmotion returns the strongest emotion and its intensity.
func (ev EmotionVector) DominantEmotion() (string, float64) {
	pairs := []struct {
		name  string
		value float64
	}{
		{"joy", ev.Joy}, {"sadness", ev.Sadness}, {"anger", ev.Anger},
		{"fear", ev.Fear}, {"trust", ev.Trust}, {"disgust", ev.Disgust},
		{"surprise", ev.Surprise}, {"attachment", ev.Attachment},
		{"resentment", ev.Resentment}, {"gratitude", ev.Gratitude},
		{"guilt", ev.Guilt}, {"longing", ev.Longing},
	}
	best := pairs[0]
	for _, p := range pairs[1:] {
		if p.value > best.value {
			best = p
		}
	}
	return best.name, best.value
}

// Contradictions returns pairs of emotions that are simultaneously high.
// e.g. attachment=0.8 + resentment=0.3 is a mild contradiction.
func (ev EmotionVector) Contradictions() []string {
	var cs []string
	if ev.Attachment > 0.5 && ev.Resentment > 0.3 {
		cs = append(cs, "attachment+resentment")
	}
	if ev.Trust > 0.5 && ev.Fear > 0.4 {
		cs = append(cs, "trust+fear")
	}
	if ev.Gratitude > 0.5 && ev.Resentment > 0.3 {
		cs = append(cs, "gratitude+resentment")
	}
	if ev.Joy > 0.5 && ev.Sadness > 0.4 {
		cs = append(cs, "joy+sadness")
	}
	if ev.Attachment > 0.5 && ev.Fear > 0.4 {
		cs = append(cs, "attachment+fear_of_loss")
	}
	return cs
}

// EmotionalResidue is a lingering emotion that persists after an event ends.
// "事件结束 ≠ 情绪结束"
type EmotionalResidue struct {
	ID          string    `json:"id"`
	Character   string    `json:"character"`
	Type        string    `json:"type"`         // disappointment, betrayal, warmth, gratitude, hurt, admiration
	SourceEvent string    `json:"source_event"` // event ID that caused it
	Target      string    `json:"target"`       // who this is directed at
	Intensity   float64   `json:"intensity"`    // 0-1, initial strength
	Current     float64   `json:"current"`      // current decayed value
	DecayRate   float64   `json:"decay_rate"`   // per day (0=permanent, 0.2=gone in 5 days)
	CreatedAt   time.Time `json:"created_at"`
}

// IsActive returns true if the residue still has emotional weight.
func (r EmotionalResidue) IsActive() bool {
	return r.Current > 0.05
}

// DecayTo advances decay to the given time and returns the new current value.
func (r *EmotionalResidue) DecayTo(at time.Time) {
	days := at.Sub(r.CreatedAt).Hours() / 24
	if days <= 0 {
		return
	}
	r.Current = r.Intensity - r.DecayRate*days
	if r.Current < 0 {
		r.Current = 0
	}
}

// UnresolvedThread is an open emotional topic between characters.
// It may be referenced, avoided, hinted at — but not resolved.
type UnresolvedThread struct {
	ID              string    `json:"id"`
	Character       string    `json:"character"`
	Topic           string    `json:"topic"`            // "未回应的告白", "背叛的沉默", "欠下的承诺"
	Involving       string    `json:"involving"`        // who this is with
	OpenedAt        string    `json:"opened_at"`        // event ID
	EmotionalWeight float64   `json:"emotional_weight"` // how heavy this feels
	Status          string    `json:"status"`           // unresolved, hinted, addressed, resolved
	LastReferenced  string    `json:"last_referenced"`  // event ID
	HintCount       int       `json:"hint_count"`       // times indirectly referenced
	CreatedAt       time.Time `json:"created_at"`
}

// DelayedReaction is an emotion that triggers after a delay.
// "当下没反应，几天后突然在意"
type DelayedReaction struct {
	ID            string        `json:"id"`
	Character     string        `json:"character"`
	TriggerEvent  string        `json:"trigger_event"` // event ID that planted the seed
	ReactionType  string        `json:"reaction_type"` // anger, sadness, realization, longing
	Intensity     float64       `json:"intensity"`
	Target        string        `json:"target"`
	DelayEvents   int           `json:"delay_events"`   // trigger after N more events
	DelayDuration time.Duration `json:"delay_duration"` // or after this much time
	Triggered     bool          `json:"triggered"`
	TriggeredAt   time.Time     `json:"triggered_at"`
	CreatedAt     time.Time     `json:"created_at"`
}

// ShouldTrigger checks if the reaction should fire now.
func (dr DelayedReaction) ShouldTrigger(eventCount int, now time.Time) bool {
	if dr.Triggered {
		return false
	}
	if dr.DelayEvents > 0 && eventCount >= dr.DelayEvents {
		return true
	}
	if dr.DelayDuration > 0 && now.After(dr.CreatedAt.Add(dr.DelayDuration)) {
		return true
	}
	return false
}

// EmotionalSnapshot is what the LLM sees in its WorldSnapshot.
type EmotionalSnapshot struct {
	DominantEmotion   string             `json:"dominant_emotion"`
	DominantIntensity float64            `json:"dominant_intensity"`
	Vector            EmotionVector      `json:"vector"`
	Contradictions    []string           `json:"contradictions"`
	ActiveResidues    []EmotionalResidue `json:"active_residues"`
	UnresolvedThreads []UnresolvedThread `json:"unresolved_threads"`
	PendingReactions  []DelayedReaction  `json:"pending_reactions"`
}
