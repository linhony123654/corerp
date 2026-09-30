package core

import (
	"regexp"
	"strings"
)

// Manifest field names and meanings preserve docs/m0/core-contract.schema.json.
const StudioPackageManifestVersion = "m0-draft-2026-09-22"
const StudioPackageSchemaHash = "sha256:0b7d979a3384df06726118c293ec3533e501cd0740f0819067e8e6191c2667a0"
const StudioPackageContentVersion = "corerp.studio-package.v1"

type PackageDependency struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type PackageManifest struct {
	SchemaVersion string              `json:"schema_version"`
	ID            string              `json:"id"`
	Kind          string              `json:"kind"`
	Version       string              `json:"version"`
	EngineAPI     string              `json:"engine_api"`
	Requires      []PackageDependency `json:"requires"`
	Optional      []PackageDependency `json:"optional"`
	Capabilities  []string            `json:"capabilities"`
	SchemaHash    string              `json:"schema_hash"`
	ContentHash   string              `json:"content_hash"`
	ContentFiles  []string            `json:"content_files"`
}

type StudioActivityRule struct {
	// DurationMinutes is the rule-bound cost of the activity; completion is
	// committed deterministically when world time passes start+duration.
	// A model can start an activity but can never declare it done itself.
	DurationMinutes int `json:"duration_minutes"`
}

// StudioBackgroundProgression is an explicit, bounded opt-in. Omitting this
// block (or setting enabled=false with no other values) keeps the world still.
// Runtime workers may only use the existing scheduler and world-time authority.
type StudioBackgroundProgression struct {
	Enabled         bool `json:"enabled"`
	StepMinutes     int  `json:"step_minutes,omitempty"`
	SchedulerBudget int  `json:"scheduler_budget,omitempty"`
}

type StudioSystemRules struct {
	NPCDailyActionBudget int `json:"npc_daily_action_budget"`
	// RPExecutionMode opts a world into bounded turn activation. Empty preserves
	// the legacy all-listeners behavior for already-authored packages.
	RPExecutionMode       string                       `json:"rp_execution_mode,omitempty"`
	MaxActiveResponders   int                          `json:"max_active_responders,omitempty"`
	BackgroundProgression *StudioBackgroundProgression `json:"background_progression,omitempty"`
	// Activities declares the world's legal activity vocabulary for NPC act
	// decisions. Codes not declared here are never legal.
	Activities map[string]StudioActivityRule `json:"activities,omitempty"`
}

// Retail entries are author references. They do not create a posting, issue a
// credential, schedule work, grant authority, or write an RP memory.
type StudioRetailCareerCatalog struct {
	Version        string                  `json:"version"`
	OrganizationID string                  `json:"organization_id"`
	Jobs           []StudioRetailCareerJob `json:"jobs"`
}

type StudioRetailCareerJob struct {
	PositionID         string                     `json:"position_id"`
	Title              string                     `json:"title"`
	OccupationID       string                     `json:"occupation_id"`
	Grade              string                     `json:"grade"`
	WageReferenceMinor int64                      `json:"wage_reference_minor"`
	RequiredCredential CredentialRequirement      `json:"required_credential"`
	Training           StudioRetailCareerTraining `json:"training"`
	WorkStartHour      int                        `json:"work_start_hour"`
	WorkEndHour        int                        `json:"work_end_hour"`
	NextGrade          string                     `json:"next_grade"`
}

type StudioRetailCareerTraining struct {
	ProgramID      string `json:"program_id"`
	MinimumMinutes int    `json:"minimum_minutes"`
	ExercisePrompt string `json:"exercise_prompt"`
}

func (c StudioRetailCareerCatalog) Validate() error {
	if c.Version != "corerp.retail-career.v1" || validateCareerIDs(c.OrganizationID) != nil || len(c.Jobs) < 1 || len(c.Jobs) > 16 {
		return NewError(CodeInvalidArgument, "invalid bounded retail career catalog")
	}
	positions, programs := map[string]bool{}, map[string]bool{}
	for _, job := range c.Jobs {
		if validateCareerIDs(job.PositionID, job.Title, job.OccupationID, job.Grade, job.NextGrade, job.RequiredCredential.Code, job.RequiredCredential.IssuerID, job.Training.ProgramID) != nil || positions[job.PositionID] || programs[job.Training.ProgramID] || job.Grade == job.NextGrade || job.WageReferenceMinor < 1 || job.WageReferenceMinor > MaxJSONSafeInteger || job.WorkStartHour < 0 || job.WorkStartHour > 22 || job.WorkEndHour <= job.WorkStartHour || job.WorkEndHour > 23 || job.Training.MinimumMinutes < 1 || job.Training.MinimumMinutes > 7*24*60 || job.Training.ExercisePrompt == "" || len(job.Training.ExercisePrompt) > 1000 {
			return NewError(CodeInvalidArgument, "invalid retail career job reference")
		}
		positions[job.PositionID], programs[job.Training.ProgramID] = true, true
	}
	return nil
}

// These are declarative values for existing owners, never code or SQL.
type StudioPackageContent struct {
	Version        string                     `json:"version"`
	NarrativeStyle *RPStyleProfile            `json:"narrative_style,omitempty"`
	SystemRules    *StudioSystemRules         `json:"system_rules,omitempty"`
	RetailCareer   *StudioRetailCareerCatalog `json:"retail_career,omitempty"`
	// ActivityLabels declares prose labels for system-package activity codes
	// (e.g. tend_accounts -> 理账). Presentation metadata only: rendering shows
	// the label when present and falls back to the raw code otherwise.
	ActivityLabels map[string]string `json:"activity_labels,omitempty"`
}

type StudioPackageBundle struct {
	Manifest PackageManifest      `json:"manifest"`
	Content  StudioPackageContent `json:"content"`
}

var packageID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,159}$`)
var packageVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// Runtime support is deliberately narrower than the exchange schema. An accepted
// manifest never implies permission to execute a plugin or grant capabilities.
func (b StudioPackageBundle) Validate() error {
	bad := func(message string) error { return NewError(CodeInvalidArgument, message) }
	m, c := b.Manifest, b.Content
	if m.SchemaVersion != StudioPackageManifestVersion || m.EngineAPI != StudioPackageManifestVersion || m.SchemaHash != StudioPackageSchemaHash {
		return bad("unsupported package manifest, engine API or schema hash")
	}
	if !packageID.MatchString(m.ID) || len(m.Version) > 80 || !packageVersion.MatchString(m.Version) {
		return bad("invalid package identity or exact version")
	}
	if m.Requires == nil || m.Optional == nil || len(m.Requires)+len(m.Optional) > 32 {
		return bad("bounded explicit dependency arrays required")
	}
	dependencies := map[string]bool{m.ID: true}
	for _, list := range [][]PackageDependency{m.Requires, m.Optional} {
		for _, d := range list {
			if !packageID.MatchString(d.ID) || len(d.Version) > 80 || !packageVersion.MatchString(d.Version) || dependencies[d.ID] {
				return bad("invalid, duplicate or self-referencing package dependency")
			}
			dependencies[d.ID] = true
		}
	}
	if c.Version != StudioPackageContentVersion {
		return bad("unsupported package content version")
	}
	capability, filename := "", ""
	switch m.Kind {
	case "system":
		capability, filename = "rules.npc.daily_budget", "system.json"
		if c.SystemRules == nil || c.NarrativeStyle != nil || c.RetailCareer != nil || len(c.ActivityLabels) != 0 || c.SystemRules.NPCDailyActionBudget < 1 || c.SystemRules.NPCDailyActionBudget > 64 {
			return bad("system package requires only bounded NPC daily action rules")
		}
		switch c.SystemRules.RPExecutionMode {
		case "":
			if c.SystemRules.MaxActiveResponders != 0 {
				return bad("legacy RP execution cannot set a responder limit")
			}
		case "deterministic", "orchestrated", "multi_agent":
			if c.SystemRules.MaxActiveResponders < 1 || c.SystemRules.MaxActiveResponders > 8 {
				return bad("RP execution responder limit must be between 1 and 8")
			}
			if c.SystemRules.RPExecutionMode == "deterministic" && c.SystemRules.MaxActiveResponders != 1 {
				return bad("deterministic RP execution requires exactly one active responder")
			}
		default:
			return bad("unsupported RP execution mode")
		}
		if progression := c.SystemRules.BackgroundProgression; progression != nil {
			if !progression.Enabled {
				if progression.StepMinutes != 0 || progression.SchedulerBudget != 0 {
					return bad("disabled background progression cannot set runtime budgets")
				}
			} else if progression.StepMinutes < 1 || progression.StepMinutes > 24*60 || progression.SchedulerBudget < 1 || progression.SchedulerBudget > 10000 {
				return bad("background progression requires a 1-1440 minute step and 1-10000 scheduler budget")
			}
		}
		for code, rule := range c.SystemRules.Activities {
			if !studioLocalKey.MatchString(code) || rule.DurationMinutes < 1 || rule.DurationMinutes > 480 {
				return bad("invalid activity rule")
			}
		}
	case "narrative":
		capability, filename = "narrative.style", "narrative.json"
		if c.NarrativeStyle == nil || c.SystemRules != nil || c.RetailCareer != nil {
			return bad("narrative package requires only presentation style and activity labels")
		}
		if err := c.NarrativeStyle.Validate(); err != nil {
			return err
		}
		if len(c.ActivityLabels) > 64 {
			return bad("too many activity labels")
		}
		for code, label := range c.ActivityLabels {
			if !studioLocalKey.MatchString(code) || strings.TrimSpace(label) == "" || len([]rune(label)) > 24 {
				return bad("invalid activity label")
			}
		}
	case "content":
		capability, filename = "content.career.retail", "content.json"
		if c.RetailCareer == nil || c.SystemRules != nil || c.NarrativeStyle != nil || len(c.ActivityLabels) != 0 {
			return bad("retail content package requires only its typed catalog")
		}
		if err := c.RetailCareer.Validate(); err != nil {
			return err
		}
	default:
		return bad("runtime supports declarative system, narrative and retail content packages only")
	}
	if len(m.Capabilities) != 1 || m.Capabilities[0] != capability {
		return bad("unknown or incompatible package capability")
	}
	if len(m.ContentFiles) != 1 || m.ContentFiles[0] != filename {
		return bad("package requires its single declared JSON content file")
	}
	hash, err := HashJSON(c)
	if err != nil {
		return err
	}
	if hash != m.ContentHash {
		return bad("package content hash differs")
	}
	encoded, err := CanonicalJSON(b)
	if err != nil {
		return err
	}
	if len(encoded) > 64*1024 {
		return bad("package exceeds 64 KiB canonical content limit")
	}
	return nil
}

// Validate the entire pinned set so a later install cannot invalidate an earlier
// optional dependency. Present optional dependencies participate in cycle checks.
func ValidateStudioPackageSet(bundles []StudioPackageBundle) error {
	bad := func(message string) error { return NewError(CodeInvalidArgument, message) }
	if len(bundles) > 32 {
		return bad("world package limit reached")
	}
	byID := map[string]StudioPackageBundle{}
	for _, b := range bundles {
		if err := b.Validate(); err != nil {
			return err
		}
		if _, exists := byID[b.Manifest.ID]; exists {
			return bad("duplicate package identity")
		}
		byID[b.Manifest.ID] = b
	}
	for _, b := range bundles {
		for i, list := range [][]PackageDependency{b.Manifest.Requires, b.Manifest.Optional} {
			for _, d := range list {
				dep, present := byID[d.ID]
				if !present && i == 1 {
					continue
				}
				if !present || dep.Manifest.Version != d.Version {
					return bad("exact package dependency missing or incompatible")
				}
			}
		}
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return bad("package dependency cycle")
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		b := byID[id]
		for _, list := range [][]PackageDependency{b.Manifest.Requires, b.Manifest.Optional} {
			for _, d := range list {
				if _, present := byID[d.ID]; present {
					if err := visit(d.ID); err != nil {
						return err
					}
				}
			}
		}
		state[id] = 2
		return nil
	}
	for _, b := range bundles {
		if err := visit(b.Manifest.ID); err != nil {
			return err
		}
	}
	return nil
}
