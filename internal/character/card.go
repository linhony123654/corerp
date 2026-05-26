package character

import (
	"os"

	"corerp/internal/core"

	"gopkg.in/yaml.v3"
)

type cardYAML struct {
	Identity struct {
		Name         string             `yaml:"name"`
		Immutable    []string           `yaml:"immutable"`
		Adaptive     map[string]float64 `yaml:"adaptive"`
		Forbidden    []string           `yaml:"forbidden"`
		Voice        voiceYAML          `yaml:"voice"`
		WritingGuide string             `yaml:"writing_guide"`
	} `yaml:"identity"`
	Goals struct {
		Primary   []goalYAML `yaml:"primary"`
		Secondary []goalYAML `yaml:"secondary"`
		Hidden    []goalYAML `yaml:"hidden"`
	} `yaml:"goals"`
}

type voiceYAML struct {
	Style  string `yaml:"style"`
	Rhythm string `yaml:"rhythm"`
}

type goalYAML struct {
	ID              string   `yaml:"id"`
	Priority        int      `yaml:"priority"`
	Condition       string   `yaml:"condition,omitempty"`
	CooldownTurns   int      `yaml:"cooldown_turns,omitempty"`
	Target          string   `yaml:"target,omitempty"`
	KnownBy         []string `yaml:"known_by,omitempty"`
	RevealCondition string   `yaml:"reveal_condition,omitempty"`
}

func Load(path string) (core.Character, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return core.Character{}, err
	}

	var raw cardYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return core.Character{}, err
	}

	char := core.Character{
		Identity: core.IdentityEnvelope{
			Name:      raw.Identity.Name,
			Immutable: raw.Identity.Immutable,
			Adaptive:  raw.Identity.Adaptive,
			Forbidden: raw.Identity.Forbidden,
			Voice: core.VoiceConfig{
				Style:  raw.Identity.Voice.Style,
				Rhythm: raw.Identity.Voice.Rhythm,
			},
			WritingGuide: raw.Identity.WritingGuide,
		},
	}
	for _, g := range raw.Goals.Primary {
		char.Goals = append(char.Goals, core.Goal{
			ID:            g.ID,
			Priority:      g.Priority,
			Type:          "primary",
			Target:        g.Target,
			Condition:     g.Condition,
			CooldownTurns: g.CooldownTurns,
		})
	}
	for _, g := range raw.Goals.Secondary {
		char.Goals = append(char.Goals, core.Goal{
			ID:            g.ID,
			Priority:      g.Priority,
			Type:          "secondary",
			Target:        g.Target,
			Condition:     g.Condition,
			CooldownTurns: g.CooldownTurns,
		})
	}
	for _, g := range raw.Goals.Hidden {
		char.Goals = append(char.Goals, core.Goal{
			ID:              g.ID,
			Priority:        g.Priority,
			Type:            "hidden",
			KnownBy:         g.KnownBy,
			RevealCondition: g.RevealCondition,
			Condition:       g.Condition,
			CooldownTurns:   g.CooldownTurns,
			Target:          g.Target,
		})
	}
	return char, nil
}

func Save(path string, char core.Character) error {
	raw := cardYAML{}
	raw.Identity.Name = char.Identity.Name
	raw.Identity.Immutable = char.Identity.Immutable
	raw.Identity.Adaptive = char.Identity.Adaptive
	raw.Identity.Forbidden = char.Identity.Forbidden
	raw.Identity.Voice.Style = char.Identity.Voice.Style
	raw.Identity.Voice.Rhythm = char.Identity.Voice.Rhythm
	raw.Identity.WritingGuide = char.Identity.WritingGuide

	for _, g := range char.Goals {
		item := goalYAML{
			ID:              g.ID,
			Priority:        g.Priority,
			Condition:       g.Condition,
			CooldownTurns:   g.CooldownTurns,
			Target:          g.Target,
			KnownBy:         g.KnownBy,
			RevealCondition: g.RevealCondition,
		}
		switch g.Type {
		case "secondary":
			raw.Goals.Secondary = append(raw.Goals.Secondary, item)
		case "hidden":
			raw.Goals.Hidden = append(raw.Goals.Hidden, item)
		default:
			raw.Goals.Primary = append(raw.Goals.Primary, item)
		}
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
