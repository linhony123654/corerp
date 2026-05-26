package importer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SillyTavernChar supports both v1 (flat) and v2 (data.*) formats.
type SillyTavernChar struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Personality    string                 `json:"personality"`
	Scenario       string                 `json:"scenario"`
	FirstMes       string                 `json:"first_mes"`
	MesExample     string                 `json:"mes_example"`
	CreatorComment string                 `json:"creatorcomment"`
	Tags           interface{}            `json:"tags"`
	Spec           string                 `json:"spec"`
	SpecVersion    string                 `json:"spec_version"`
	Data           map[string]interface{} `json:"data"`
}

// CharacterYAML is the CoreRP character card format.
type CharacterYAML struct {
	Identity    IdentityYAML `yaml:"identity"`
	Goals       GoalsYAML    `yaml:"goals"`
	OpeningLine string       `yaml:"opening_line,omitempty"`
}

type CardKind string

const (
	CardKindSingle   CardKind = "single_character"
	CardKindEnsemble CardKind = "ensemble_world"
)

type CastIndexYAML struct {
	Kind              CardKind         `yaml:"kind"`
	WorldName         string           `yaml:"world_name"`
	PrimaryCharacter  string           `yaml:"primary_character"`
	GeneratedAtImport []CastMemberYAML `yaml:"generated_at_import"`
	SecondaryCast     []CastMemberYAML `yaml:"secondary_cast,omitempty"`
}

type CastMemberYAML struct {
	Name       string `yaml:"name"`
	Role       string `yaml:"role"`
	SourceType string `yaml:"source_type"`
	File       string `yaml:"file,omitempty"`
}

type ImportBundle struct {
	Kind             CardKind
	PrimaryCharacter string
	Characters       map[string]CharacterYAML
	World            WorldYAML
	CastIndex        CastIndexYAML
}

type ImportPreview struct {
	Kind             CardKind
	PrimaryCharacter string
	WorldName        string
	CharacterNames   []string
}

// WorldYAML is the CoreRP world setting format (Canon Layer).
type WorldYAML struct {
	Name      string       `yaml:"name"`
	CoreRules string       `yaml:"core_rules"`
	Scene     SceneYAML    `yaml:"scene"`
	Ontology  OntologyYAML `yaml:"ontology,omitempty"`
}

type SceneYAML struct {
	Location    string   `yaml:"location"`
	TimeOfDay   string   `yaml:"time_of_day"`
	Weather     string   `yaml:"weather"`
	Characters  []string `yaml:"characters"`
	Description string   `yaml:"description"`
}

type OntologyYAML struct {
	Characters []EntityEntry `yaml:"characters,omitempty"`
	Locations  []EntityEntry `yaml:"locations,omitempty"`
	Factions   []EntityEntry `yaml:"factions,omitempty"`
	Items      []EntityEntry `yaml:"items,omitempty"`
	Lore       []EntityEntry `yaml:"lore,omitempty"`
	Settings   []EntityEntry `yaml:"settings,omitempty"`
	Events     []EventEntry  `yaml:"events,omitempty"`
	Timelines  []EntityEntry `yaml:"timelines,omitempty"`
}

type EntityEntry struct {
	Name    string `yaml:"name"`
	Keys    string `yaml:"keys,omitempty"`
	Content string `yaml:"content"`
}

type FactEntry struct {
	Subject    string  `yaml:"subject"`
	Predicate  string  `yaml:"predicate"`
	Object     string  `yaml:"object"`
	Confidence float64 `yaml:"confidence"`
}

type EventEntry struct {
	Name    string `yaml:"name"`
	Arc     string `yaml:"arc,omitempty"`
	Keys    string `yaml:"keys,omitempty"`
	Content string `yaml:"content"`
}

// WorldBookEntry is the raw parsed form of a character_book entry.
type WorldBookEntry struct {
	Type    string `yaml:"type"`
	Name    string `yaml:"name"`
	Keys    string `yaml:"keys"`
	Content string `yaml:"content"`
}

type IdentityYAML struct {
	Name      string             `yaml:"name"`
	Immutable []string           `yaml:"immutable"`
	Adaptive  map[string]float64 `yaml:"adaptive"`
	Forbidden []string           `yaml:"forbidden"`
	Voice     VoiceYAML          `yaml:"voice"`
}

type VoiceYAML struct {
	Style  string `yaml:"style"`
	Rhythm string `yaml:"rhythm"`
}

type GoalsYAML struct {
	Primary   []GoalYAML `yaml:"primary"`
	Secondary []GoalYAML `yaml:"secondary"`
	Hidden    []GoalYAML `yaml:"hidden"`
}

type GoalYAML struct {
	ID              string   `yaml:"id"`
	Priority        int      `yaml:"priority"`
	Target          string   `yaml:"target,omitempty"`
	Condition       string   `yaml:"condition"`
	KnownBy         []string `yaml:"known_by,omitempty"`
	RevealCondition string   `yaml:"reveal_condition,omitempty"`
}

// ImportPNG extracts SillyTavern PNG and converts to CoreRP YAMLs.
// Returns (characterPath, worldPath, error).
func ImportPNG(srcPath, dstDir string) (string, string, error) {
	return ImportPNGWithMode(srcPath, dstDir, "auto")
}

func ImportPNGWithMode(srcPath, dstDir, mode string) (string, string, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return "", "", fmt.Errorf("open png: %w", err)
	}
	defer f.Close()

	charaB64, err := extractCharaChunk(f)
	if err != nil {
		return "", "", fmt.Errorf("extract chara: %w", err)
	}

	jsonBytes, err := base64.StdEncoding.DecodeString(charaB64)
	if err != nil {
		return "", "", fmt.Errorf("decode base64: %w", err)
	}

	var st SillyTavernChar
	if err := json.Unmarshal(jsonBytes, &st); err != nil {
		return "", "", fmt.Errorf("parse json: %w", err)
	}

	bundle := ConvertBundleWithMode(st, mode)

	base := filepath.Base(srcPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	charPath := filepath.Join(dstDir, name+".yml")
	worldDir := filepath.Join(dstDir, "..", "worlds", name)
	charPath, wp, err := writeImportBundle(dstDir, worldDir, name, bundle)
	if err != nil {
		return "", "", err
	}
	return charPath, wp, nil
}

// ImportJSON reads a SillyTavern JSON card and converts to CoreRP YAMLs.
func ImportJSON(srcPath, dstDir string) (string, string, error) {
	return ImportJSONWithMode(srcPath, dstDir, "auto")
}

func ImportJSONWithMode(srcPath, dstDir, mode string) (string, string, error) {
	jsonBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return "", "", fmt.Errorf("read json: %w", err)
	}

	var st SillyTavernChar
	if err := json.Unmarshal(jsonBytes, &st); err != nil {
		return "", "", fmt.Errorf("parse json: %w", err)
	}

	bundle := ConvertBundleWithMode(st, mode)

	base := filepath.Base(srcPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	worldDir := filepath.Join(dstDir, "..", "worlds", name)
	charPath, wp, err := writeImportBundle(dstDir, worldDir, name, bundle)
	if err != nil {
		return "", "", err
	}
	return charPath, wp, nil
}

func writeImportBundle(dstDir, worldDir, baseName string, bundle ImportBundle) (string, string, error) {
	primaryFile := baseName + ".yml"
	if bundle.Kind == CardKindEnsemble {
		primaryFile = sanitizeFileComponent(bundle.PrimaryCharacter) + ".yml"
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return "", "", fmt.Errorf("create character dir: %w", err)
	}

	for name, charYAML := range bundle.Characters {
		fileName := sanitizeFileComponent(name) + ".yml"
		if bundle.Kind == CardKindSingle && name == bundle.PrimaryCharacter {
			fileName = primaryFile
		}
		charPath := filepath.Join(dstDir, fileName)
		charOut, _ := yaml.Marshal(charYAML)
		if err := os.WriteFile(charPath, charOut, 0644); err != nil {
			return "", "", fmt.Errorf("write character yaml: %w", err)
		}
		for i := range bundle.CastIndex.GeneratedAtImport {
			if bundle.CastIndex.GeneratedAtImport[i].Name == name {
				bundle.CastIndex.GeneratedAtImport[i].File = fileName
			}
		}
	}

	wp, err := writeWorldDir(worldDir, bundle.World)
	if err != nil {
		return "", "", fmt.Errorf("write world dir: %w", err)
	}

	if bundle.Kind == CardKindEnsemble {
		indexOut, _ := yaml.Marshal(bundle.CastIndex)
		if err := os.WriteFile(filepath.Join(worldDir, "cast_index.yml"), indexOut, 0644); err != nil {
			return "", "", fmt.Errorf("write cast index: %w", err)
		}
	}

	return filepath.Join(dstDir, primaryFile), wp, nil
}

// writeWorldDir creates the three-layer world directory per architecture spec.
func writeWorldDir(dir string, w WorldYAML) (string, error) {
	canonDir := filepath.Join(dir, "canon")
	scenesDir := filepath.Join(dir, "scenes")
	os.MkdirAll(canonDir, 0755)
	os.MkdirAll(scenesDir, 0755)

	// 1. world.yml — meta + core_rules (compact, <50 lines)
	worldFile := filepath.Join(dir, "world.yml")
	worldData, _ := yaml.Marshal(map[string]interface{}{
		"meta": map[string]string{
			"name":    w.Name,
			"version": "1.0",
			"source":  "sillytavern_import",
		},
		"core_rules": w.CoreRules,
	})
	os.WriteFile(worldFile, worldData, 0644)

	// 2. canon/ontology.yml — entities with id fields
	ontoFile := filepath.Join(canonDir, "ontology.yml")
	ontoData, _ := yaml.Marshal(map[string]interface{}{"ontology": w.Ontology})
	os.WriteFile(ontoFile, ontoData, 0644)

	// 3. canon/facts.yml — immutable facts (subject-predicate-object)
	factsFile := filepath.Join(canonDir, "facts.yml")
	facts := extractFacts(w.Ontology)
	// Add scene-derived facts if too few
	if len(facts) < 5 {
		facts = append(facts, extractSceneFacts(w)...)
	}
	factsData, _ := yaml.Marshal(map[string]interface{}{"facts": facts})
	os.WriteFile(factsFile, factsData, 0644)

	// 4. scenes/default.yml — runtime scene state
	sceneFile := filepath.Join(scenesDir, "default.yml")
	sceneData, _ := yaml.Marshal(map[string]interface{}{"scene": w.Scene})
	os.WriteFile(sceneFile, sceneData, 0644)

	return worldFile, nil
}

// extractSceneFacts generates basic facts from the scene.
func extractSceneFacts(w WorldYAML) []FactEntry {
	var facts []FactEntry
	if w.Scene.Location != "" && w.Scene.Location != "未知地点" {
		facts = append(facts, FactEntry{
			Subject: "场景", Predicate: "地点", Object: w.Scene.Location, Confidence: 1.0,
		})
	}
	if w.Scene.Weather != "" && w.Scene.Weather != "未知天气" {
		facts = append(facts, FactEntry{
			Subject: "场景", Predicate: "天气", Object: w.Scene.Weather, Confidence: 1.0,
		})
	}
	for _, c := range w.Scene.Characters {
		facts = append(facts, FactEntry{
			Subject: "场景", Predicate: "在场角色", Object: c, Confidence: 1.0,
		})
	}
	return facts
}

// extractFacts converts settings and lore entries into FactEntries.
func extractFacts(ont OntologyYAML) []FactEntry {
	var facts []FactEntry
	for _, e := range ont.Settings {
		facts = append(facts, FactEntry{
			Subject:    cleanFactSubject(e.Name),
			Predicate:  "体系规则",
			Object:     e.Content,
			Confidence: 1.0,
		})
	}
	for _, e := range ont.Lore {
		facts = append(facts, FactEntry{
			Subject:    cleanFactSubject(e.Name),
			Predicate:  "世界法则",
			Object:     e.Content,
			Confidence: 1.0,
		})
	}
	return facts
}

func cleanFactSubject(name string) string {
	s := name
	s = strings.TrimPrefix(s, "[概念] ")
	s = strings.TrimPrefix(s, "[设定] ")
	s = strings.TrimPrefix(s, "[体系] ")
	s = strings.TrimPrefix(s, "[规则] ")
	s = strings.TrimPrefix(s, "[其他] ")
	s = strings.TrimPrefix(s, "[社会规则] ")
	return s
}

// Convert transforms SillyTavern JSON into CoreRP CharacterYAML + WorldYAML.
// Supports v1 (flat) and v2 (data.*) formats.
func Convert(st SillyTavernChar) (CharacterYAML, WorldYAML) {
	bundle := ConvertBundle(st)
	return bundle.Characters[bundle.PrimaryCharacter], bundle.World
}

func PreviewBundle(st SillyTavernChar, forceMode string) ImportPreview {
	bundle := ConvertBundleWithMode(st, forceMode)
	var names []string
	for name := range bundle.Characters {
		names = append(names, name)
	}
	sort.Strings(names)
	return ImportPreview{
		Kind:             bundle.Kind,
		PrimaryCharacter: bundle.PrimaryCharacter,
		WorldName:        bundle.World.Name,
		CharacterNames:   names,
	}
}

func ConvertBundle(st SillyTavernChar) ImportBundle {
	return ConvertBundleWithMode(st, "auto")
}

func ConvertBundleWithMode(st SillyTavernChar, forceMode string) ImportBundle {
	// Normalize v2 data.* into top-level fields
	if st.Spec == "chara_card_v2" || st.Spec == "chara_card_v3" {
		if v, ok := st.Data["name"].(string); ok && v != "" {
			st.Name = v
		}
		if v, ok := st.Data["description"].(string); ok && v != "" {
			st.Description = v
		}
		if v, ok := st.Data["personality"].(string); ok && v != "" {
			st.Personality = v
		}
		if v, ok := st.Data["scenario"].(string); ok && v != "" {
			st.Scenario = v
		}
		if v, ok := st.Data["first_mes"].(string); ok && v != "" {
			st.FirstMes = v
		}
		if v, ok := st.Data["mes_example"].(string); ok && v != "" {
			st.MesExample = v
		}
		if v, ok := st.Data["creator_notes"].(string); ok && v != "" {
			st.CreatorComment = v
		}
	}

	// Build text pool for inference: prefer personality/description,
	// fall back to first_mes (for cards that put everything there).
	textPool := st.Personality + ". " + st.Description + ". " + st.Scenario
	if textPool == ". . " && st.FirstMes != "" {
		textPool = st.FirstMes
	}

	book := extractCharacterBook(st.Data)
	// Enrich inference pool with character entries if direct fields are empty
	allChars := filterWorldBookByType(book, "character")
	if len(allChars) > 0 {
		for _, c := range allChars {
			textPool += ". " + c.Content
		}
	}

	// Try harder: extract immutable from character book entries
	immutableTraits := extractImmutable(textPool)
	if len(immutableTraits) == 0 && len(allChars) > 0 {
		// Use first character's trait-like lines as immutable
		firstCharContent := strings.ReplaceAll(allChars[0].Content, "```yaml", "")
		firstCharContent = strings.ReplaceAll(firstCharContent, "```", "")
		for _, line := range strings.Split(firstCharContent, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, ":") && len(line) > 10 && len(line) < 150 {
				immutableTraits = append(immutableTraits, line)
			}
		}
	}
	// Ensure minimum viable immutable
	if len(immutableTraits) == 0 {
		immutableTraits = []string{"角色设定详见世界书"}
	}

	charYAML := CharacterYAML{
		Identity: IdentityYAML{
			Name:      st.Name,
			Immutable: immutableTraits,
			Adaptive: map[string]float64{
				"trust":    float64(inferTrust(st.Scenario, textPool)),
				"intimacy": 1.0,
				"fear":     float64(inferFear(st.Scenario, textPool)),
			},
			Forbidden: defaultForbidden(),
			Voice: VoiceYAML{
				Style:  inferStyle(st.MesExample, textPool),
				Rhythm: inferRhythm(st.MesExample),
			},
		},
		Goals: GoalsYAML{
			Primary:   inferPrimaryGoals(st.Scenario, textPool),
			Secondary: nil,
			Hidden:    inferHiddenGoals(st.Scenario, textPool),
		},
		OpeningLine: cleanText(st.FirstMes),
	}

	worldYAML := BuildWorldYAML(st, book)
	kind := detectCardKind(st, book)
	switch forceMode {
	case "single":
		kind = CardKindSingle
	case "ensemble":
		kind = CardKindEnsemble
	}
	if kind == CardKindSingle {
		return ImportBundle{
			Kind:             kind,
			PrimaryCharacter: st.Name,
			Characters:       map[string]CharacterYAML{st.Name: charYAML},
			World:            worldYAML,
			CastIndex: CastIndexYAML{
				Kind:             kind,
				WorldName:        worldYAML.Name,
				PrimaryCharacter: st.Name,
				GeneratedAtImport: []CastMemberYAML{{
					Name:       st.Name,
					Role:       "primary",
					SourceType: "card",
				}},
			},
		}
	}

	return buildEnsembleBundle(st, worldYAML, book, charYAML)
}

func detectCardKind(st SillyTavernChar, book []WorldBookEntry) CardKind {
	charEntries := filterWorldBookByType(book, "character")
	if len(charEntries) < 3 {
		return CardKindSingle
	}
	lowerName := strings.ToLower(st.Name)
	worldish := containsAny(lowerName, []string{"红楼梦", "世界", "群像", "合集", "完整版"})
	if worldish || len(charEntries) >= 5 {
		return CardKindEnsemble
	}
	return CardKindSingle
}

func buildEnsembleBundle(st SillyTavernChar, world WorldYAML, book []WorldBookEntry, fallback CharacterYAML) ImportBundle {
	candidates := rankCastCandidates(st, book)
	if len(candidates) == 0 {
		return ImportBundle{
			Kind:             CardKindSingle,
			PrimaryCharacter: st.Name,
			Characters:       map[string]CharacterYAML{st.Name: fallback},
			World:            world,
		}
	}

	primary := candidates[0]
	characters := make(map[string]CharacterYAML)
	index := CastIndexYAML{
		Kind:              CardKindEnsemble,
		WorldName:         world.Name,
		PrimaryCharacter:  primary.Name,
		GeneratedAtImport: []CastMemberYAML{},
	}

	for i, cand := range candidates {
		if i >= 6 {
			index.SecondaryCast = append(index.SecondaryCast, CastMemberYAML{Name: cand.Name, Role: "secondary", SourceType: cand.SourceType})
			continue
		}
		characters[cand.Name] = buildCharacterFromEntry(cand, st)
		role := "primary_cast"
		if cand.Name == primary.Name {
			role = "primary"
		}
		index.GeneratedAtImport = append(index.GeneratedAtImport, CastMemberYAML{Name: cand.Name, Role: role, SourceType: cand.SourceType})
	}

	if len(world.Scene.Characters) == 0 {
		world.Scene.Characters = []string{primary.Name, "玩家"}
	} else {
		world.Scene.Characters = normalizeImportedSceneCharacters(world.Scene.Characters, primary.Name)
	}

	return ImportBundle{
		Kind:             CardKindEnsemble,
		PrimaryCharacter: primary.Name,
		Characters:       characters,
		World:            world,
		CastIndex:        index,
	}
}

type castCandidate struct {
	Name       string
	Content    string
	Aliases    []string
	SourceType string
	Score      int
}

func rankCastCandidates(st SillyTavernChar, book []WorldBookEntry) []castCandidate {
	charEntries := filterWorldBookByType(book, "character")
	corpus := st.Name + "\n" + st.Description + "\n" + st.Scenario + "\n" + st.FirstMes
	for _, e := range charEntries {
		corpus += "\n" + e.Content
	}
	type aggregate struct {
		name       string
		aliases    []string
		content    []string
		score      int
		supporters int
	}
	var groups []*aggregate
	for _, e := range charEntries {
		name := normalizeEntryName(e.Name, e.Keys)
		if name == "" {
			continue
		}
		aliases := buildCandidateAliases(name, e.Keys)
		meaningfulAliases := filterMeaningfulAliases(aliases)
		canonicalName := chooseCanonicalName(name, aliases)
		scoringAliases := buildScoringAliases(canonicalName, meaningfulAliases)
		score := 0
		if len([]rune(e.Content)) > 80 {
			score += 2
		}
		if containsAny(e.Content, []string{"性格", "外貌", "关系", "身份", "喜欢", "讨厌", "秘密", "目标"}) {
			score += 3
		}
		if idx, matched := earliestAliasIndex(st.FirstMes, scoringAliases); matched {
			score += 6
			score += openingPositionBonus(idx)
		}
		if containsAnyAlias(st.Description+"\n"+st.Scenario, scoringAliases) {
			score += 3
		}
		if mentions := countAliasOccurrences(corpus, scoringAliases); mentions > 0 {
			if mentions > 5 {
				mentions = 5
			}
			score += mentions * 2
		}
		score -= roleLikeNamePenalty(canonicalName)

		merged := false
		for _, group := range groups {
			if !shouldMergeCandidate(group.name, group.aliases, canonicalName, aliases) {
				continue
			}
			group.name = chooseCanonicalName(group.name, append(group.aliases, aliases...))
			group.aliases = mergeAliases(group.aliases, aliases)
			group.content = appendUniqueText(group.content, e.Content)
			group.score += score
			group.supporters++
			merged = true
			break
		}
		if merged {
			continue
		}
		groups = append(groups, &aggregate{
			name:       canonicalName,
			aliases:    aliases,
			content:    []string{e.Content},
			score:      score,
			supporters: 1,
		})
	}

	var out []castCandidate
	for _, group := range groups {
		score := group.score
		if group.supporters > 1 {
			score += (group.supporters - 1) * 2
		}
		score += countAliasEntryMentions(charEntries, buildScoringAliases(group.name, group.aliases))
		out = append(out, castCandidate{
			Name:       group.name,
			Content:    strings.Join(group.content, "\n\n"),
			Aliases:    mergeAliases(nil, group.aliases),
			SourceType: "character_book",
			Score:      score,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Name < out[j].Name
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func buildCharacterFromEntry(c castCandidate, st SillyTavernChar) CharacterYAML {
	text := cleanText(c.Content)
	immutable := extractImmutable(text)
	if len(immutable) == 0 {
		immutable = []string{truncateForTrait(text)}
	}
	return CharacterYAML{
		Identity: IdentityYAML{
			Name:      c.Name,
			Immutable: immutable,
			Adaptive: map[string]float64{
				"trust":    float64(inferTrust(st.Scenario, text)),
				"intimacy": 1.0,
				"fear":     float64(inferFear(st.Scenario, text)),
			},
			Forbidden: defaultForbidden(),
			Voice: VoiceYAML{
				Style:  inferStyle(st.MesExample, text),
				Rhythm: inferRhythm(st.MesExample),
			},
		},
		Goals: GoalsYAML{
			Primary: inferPrimaryGoals(st.Scenario, text),
			Hidden:  inferHiddenGoals(st.Scenario, text),
		},
	}
}

func normalizeImportedSceneCharacters(chars []string, primary string) []string {
	var out []string
	seen := make(map[string]bool)
	if primary != "" {
		out = append(out, primary)
		seen[primary] = true
	}
	for _, name := range chars {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		out = append(out, name)
		seen[name] = true
	}
	hasPlayer := false
	for _, name := range out {
		if name == "玩家" || name == "用户" {
			hasPlayer = true
			break
		}
	}
	if !hasPlayer {
		out = append(out, "玩家")
	}
	return out
}

func normalizeEntryName(name, keys string) string {
	raw := strings.TrimSpace(name)
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "[]")
	name = strings.TrimLeft(name, "-*• \t")
	for _, prefix := range []string{
		"角色内心 -", "角色内心-", "内心 -", "内心-",
		"角色 -", "角色-", "人物 -", "人物-", "NPC -", "NPC-",
		"角色]", "人物]", "NPC]", "角色", "人物", "NPC", "[角色", "[人物", "[NPC",
	} {
		name = strings.TrimSpace(strings.TrimPrefix(name, prefix))
	}
	for _, suffix := range []string{"-内心世界", "内心世界"} {
		name = strings.TrimSpace(strings.TrimSuffix(name, suffix))
	}
	if strings.Contains(raw, "内心") && strings.Contains(name, "-") {
		name = strings.TrimSpace(strings.SplitN(name, "-", 2)[0])
	}
	name = strings.TrimLeft(name, "-*• \t")
	if name != "" {
		return name
	}
	for _, key := range strings.Split(keys, ",") {
		key = strings.TrimSpace(key)
		if key != "" {
			return key
		}
	}
	return ""
}

func countOccurrences(text, name string) int {
	if name == "" {
		return 0
	}
	return strings.Count(text, name)
}

func buildCandidateAliases(name, keys string) []string {
	seen := make(map[string]bool)
	var aliases []string
	add := func(v string) {
		v = normalizeEntryName(v, "")
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		aliases = append(aliases, v)
	}
	add(name)
	for _, key := range strings.Split(keys, ",") {
		add(key)
	}
	return aliases
}

func mergeAliases(base []string, extra []string) []string {
	seen := make(map[string]bool, len(base)+len(extra))
	var out []string
	for _, alias := range append(base, extra...) {
		alias = normalizeEntryName(alias, "")
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		out = append(out, alias)
	}
	return out
}

func appendUniqueText(base []string, extra string) []string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return base
	}
	for _, existing := range base {
		if existing == extra {
			return base
		}
	}
	return append(base, extra)
}

func filterMeaningfulAliases(aliases []string) []string {
	var out []string
	for _, alias := range aliases {
		if isMeaningfulAlias(alias) {
			out = append(out, alias)
		}
	}
	if len(out) > 0 {
		return out
	}
	if len(aliases) > 0 {
		return []string{aliases[0]}
	}
	return nil
}

func buildScoringAliases(canonicalName string, aliases []string) []string {
	canonicalName = normalizeEntryName(canonicalName, "")
	if canonicalName == "" {
		return nil
	}
	seen := map[string]bool{canonicalName: true}
	out := []string{canonicalName}
	for _, alias := range aliases {
		alias = normalizeEntryName(alias, "")
		if alias == "" || seen[alias] || isRoleLikeAlias(alias) {
			continue
		}
		if alias == canonicalName ||
			strings.Contains(canonicalName, alias) ||
			strings.Contains(alias, canonicalName) ||
			(sharesLikelySurname(canonicalName, alias) && len([]rune(alias)) >= 2) {
			seen[alias] = true
			out = append(out, alias)
		}
	}
	return out
}

func isMeaningfulAlias(alias string) bool {
	alias = normalizeEntryName(alias, "")
	if alias == "" {
		return false
	}
	if isGenericAlias(alias) {
		return false
	}
	return len([]rune(alias)) >= 2
}

func isGenericAlias(alias string) bool {
	if alias == "" {
		return true
	}
	generic := map[string]struct{}{
		"你": {}, "我": {}, "他": {}, "她": {}, "它": {},
		"爷": {}, "哥": {}, "姐": {}, "嫂": {}, "叔": {}, "婶": {}, "娘": {}, "爹": {},
		"老爷": {}, "太太": {}, "奶奶": {}, "姑娘": {}, "小姐": {}, "夫人": {}, "公子": {},
		"少爷": {}, "老内相": {}, "王爷": {}, "道人": {}, "道士": {}, "先生": {},
		"哥哥": {}, "妹妹": {}, "姐姐": {}, "弟弟": {}, "叔叔": {}, "婶子": {},
		"大爷": {}, "二爷": {}, "三爷": {}, "大哥": {}, "二哥": {}, "大姐": {}, "二姐": {},
		"内心": {}, "心理": {}, "情感": {}, "情绪": {}, "独白": {}, "回忆": {},
		"众人": {}, "诸公": {}, "众客": {}, "那道": {}, "一道": {}, "那府": {}, "那边": {},
	}
	if _, ok := generic[alias]; ok {
		return true
	}
	if strings.HasPrefix(alias, "第") && strings.HasSuffix(alias, "章") {
		return true
	}
	return false
}

func isRoleLikeAlias(alias string) bool {
	for _, marker := range []string{"之妻", "之女", "之子", "母亲", "父亲", "妈妈", "爸爸", "外祖母", "祖母", "祖父", "家的", "媳妇", "老婆", "太爷", "丫头"} {
		if strings.Contains(alias, marker) {
			return true
		}
	}
	return false
}

func shouldMergeCandidate(currentName string, currentAliases []string, nextName string, nextAliases []string) bool {
	if currentName == nextName {
		return true
	}
	current := buildScoringAliases(currentName, filterMeaningfulAliases(currentAliases))
	next := buildScoringAliases(nextName, filterMeaningfulAliases(nextAliases))
	overlap := 0
	set := make(map[string]bool, len(current))
	for _, alias := range current {
		set[alias] = true
	}
	for _, alias := range next {
		if set[alias] {
			overlap++
		}
	}
	if overlap > 0 {
		return true
	}
	for _, alias := range current {
		if alias == nextName {
			return true
		}
	}
	for _, alias := range next {
		if alias == currentName {
			return true
		}
	}
	return false
}

func chooseCanonicalName(name string, aliases []string) string {
	best := normalizeEntryName(name, "")
	bestScore := canonicalNameScore(best)
	for _, alias := range aliases {
		alias = normalizeEntryName(alias, "")
		if alias == "" || isGenericAlias(alias) {
			continue
		}
		score := canonicalNameScore(alias)
		if score > bestScore || (score == bestScore && alias < best) {
			best = alias
			bestScore = score
		}
	}
	return best
}

func canonicalNameScore(name string) int {
	name = normalizeEntryName(name, "")
	if name == "" {
		return -1
	}
	score := 0
	if hasLikelyChineseSurname(name) {
		score += 30
	}
	runes := []rune(name)
	if l := len(runes); l <= 6 {
		score += l * 3
	} else {
		score += 18 - (l-6)*2
	}
	for _, marker := range []string{"公子", "主人", "老爷", "太太", "奶奶", "姑娘", "姐姐", "妹妹", "哥哥"} {
		if strings.Contains(name, marker) {
			score -= 8
		}
	}
	for _, marker := range []string{"姐儿", "哥儿", "丫头"} {
		if strings.Contains(name, marker) {
			score -= 10
		}
	}
	if strings.HasPrefix(name, "小") || strings.HasPrefix(name, "老") {
		score -= 4
	}
	score -= roleLikeNamePenalty(name)
	return score
}

func roleLikeNamePenalty(name string) int {
	if name == "" {
		return 0
	}
	penalty := 0
	for _, marker := range []string{"之妻", "之女", "之子", "母亲", "父亲", "外祖母", "祖母", "祖父", "家的", "媳妇", "老婆"} {
		if strings.Contains(name, marker) {
			penalty += 10
		}
	}
	for _, marker := range []string{"太爷", "奶奶", "丫头", "姐儿", "哥儿"} {
		if strings.Contains(name, marker) {
			penalty += 6
		}
	}
	return penalty
}

func hasLikelyChineseSurname(name string) bool {
	runes := []rune(name)
	if len(runes) < 2 {
		return false
	}
	const commonSurnames = "赵钱孙李周吴郑王冯陈褚卫蒋沈韩杨朱秦尤许何吕施张孔曹严华金魏陶姜戚谢邹喻柏水窦章云苏潘葛奚范彭郎鲁韦昌马苗凤花方俞任袁柳鲍史唐费廉岑薛雷贺倪汤滕殷罗毕郝邬安常乐于时傅皮卞齐康伍余元卜顾孟平黄和穆萧尹姚邵湛汪祁毛禹狄米贝明臧计伏成戴谈宋茅庞熊纪舒屈项祝董梁杜阮蓝闵席季麻强贾路娄危江童颜郭梅盛林钟徐邱骆高夏蔡田胡凌霍虞万支柯昝管卢莫经房裘缪干解应宗丁宣贲邓郁单杭洪包诸左石崔吉钮龚程嵇邢滑裴陆荣翁荀羊於惠甄曲家封芮羿储靳汲邴糜松井段富巫乌焦巴弓牧隗山谷车侯宓蓬全郗班仰秋仲伊宫宁仇栾暴甘厉戎祖武符刘景詹束龙叶司温庄晏柴瞿阎充慕连习宦艾鱼容向古易慎戈廖庾终暨居衡步都耿满弘匡国文寇广禄阙东欧殳沃利蔚越夔隆师巩厍聂晁勾敖融冷辛阚那简饶空曾毋沙乜养鞠须丰巢关蒯相查后荆红游竺权逯盖益桓公"
	return strings.ContainsRune(commonSurnames, runes[0])
}

func sharesLikelySurname(left, right string) bool {
	lr := []rune(left)
	rr := []rune(right)
	if len(lr) < 2 || len(rr) < 2 {
		return false
	}
	return lr[0] == rr[0] && hasLikelyChineseSurname(left)
}

func containsAnyAlias(text string, aliases []string) bool {
	for _, alias := range aliases {
		if alias != "" && strings.Contains(text, alias) {
			return true
		}
	}
	return false
}

func countAliasOccurrences(text string, aliases []string) int {
	total := 0
	for _, alias := range aliases {
		total += countOccurrences(text, alias)
	}
	return total
}

func countAliasEntryMentions(entries []WorldBookEntry, aliases []string) int {
	total := 0
	for _, entry := range entries {
		if containsAnyAlias(entry.Content, aliases) {
			total++
		}
	}
	if total > 20 {
		return 20
	}
	return total
}

func earliestAliasIndex(text string, aliases []string) (int, bool) {
	best := -1
	for _, alias := range aliases {
		if alias == "" {
			continue
		}
		if idx := strings.Index(text, alias); idx >= 0 && (best == -1 || idx < best) {
			best = idx
		}
	}
	return best, best >= 0
}

func openingPositionBonus(idx int) int {
	switch {
	case idx <= 12:
		return 4
	case idx <= 32:
		return 2
	default:
		return 1
	}
}

func sanitizeFileComponent(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, " ", "_")
	if name == "" {
		return "character"
	}
	return name
}

func truncateForTrait(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	rs := []rune(text)
	if len(rs) > 48 {
		return string(rs[:48]) + "..."
	}
	if text == "" {
		return "角色设定详见世界书"
	}
	return text
}

// BuildWorldYAML assembles Canon Layer data from character_book entries.
func BuildWorldYAML(st SillyTavernChar, book []WorldBookEntry) WorldYAML {
	world := WorldYAML{
		Name:  st.Name,
		Scene: inferScene(st, book),
	}

	// CoreRules: system_prompt + world-building settings/concepts
	var rules []string
	if sp := extractSystemPrompt(st.Data); sp != "" {
		rules = append(rules, cleanText(sp))
	}
	// Collect setting/lore entries that define world rules
	for _, e := range book {
		if e.Type == "setting" || e.Type == "lore" {
			name := strings.ToLower(e.Name)
			// World-building content: game systems, rules, core concepts
			if strings.Contains(name, "世界观") || strings.Contains(name, "体系") ||
				strings.Contains(name, "规则") || strings.Contains(name, "设定") ||
				strings.Contains(name, "系统") || strings.Contains(name, "货币") ||
				strings.Contains(name, "等级") || strings.Contains(name, "社会") {
				rules = append(rules, "【"+e.Name+"】 "+e.Content)
			}
		}
	}
	if len(rules) == 0 && st.FirstMes != "" {
		// Last resort: extract only the rule-like sentences from first_mes
		ruleLines := extractRuleLines(st.FirstMes)
		if len(ruleLines) > 0 {
			rules = append(rules, cleanText(strings.Join(ruleLines, " ")))
		}
	}
	world.CoreRules = strings.Join(rules, "\n\n")

	for _, e := range book {
		switch e.Type {
		case "character":
			world.Ontology.Characters = append(world.Ontology.Characters, EntityEntry{
				Name: e.Name, Keys: e.Keys, Content: e.Content,
			})
		case "setting":
			// System/ability/class entries → Settings
			world.Ontology.Settings = append(world.Ontology.Settings, EntityEntry{
				Name: e.Name, Keys: e.Keys, Content: e.Content,
			})
		case "faction":
			world.Ontology.Factions = append(world.Ontology.Factions, EntityEntry{
				Name: e.Name, Keys: e.Keys, Content: e.Content,
			})
		case "location":
			world.Ontology.Locations = append(world.Ontology.Locations, EntityEntry{
				Name: e.Name, Keys: e.Keys, Content: e.Content,
			})
		case "item":
			world.Ontology.Items = append(world.Ontology.Items, EntityEntry{
				Name: e.Name, Keys: e.Keys, Content: e.Content,
			})
		case "lore":
			world.Ontology.Lore = append(world.Ontology.Lore, EntityEntry{
				Name: e.Name, Keys: e.Keys, Content: e.Content,
			})
		case "event":
			world.Ontology.Events = append(world.Ontology.Events, EventEntry{
				Name: e.Name, Arc: extractArc(e.Name), Keys: e.Keys, Content: e.Content,
			})
		case "timeline":
			world.Ontology.Timelines = append(world.Ontology.Timelines, EntityEntry{
				Name: e.Name, Keys: e.Keys, Content: e.Content,
			})
		}
	}
	return world
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractCharaChunk reads PNG tEXt chunks looking for key="chara".
func extractCharaChunk(f *os.File) (string, error) {
	header := make([]byte, 8)
	if _, err := f.Read(header); err != nil {
		return "", err
	}
	if string(header) != "\x89PNG\r\n\x1a\n" {
		return "", fmt.Errorf("not a valid PNG file")
	}

	for {
		lenBuf := make([]byte, 4)
		if _, err := f.Read(lenBuf); err != nil {
			return "", fmt.Errorf("read chunk len: %w", err)
		}
		length := uint32(lenBuf[0])<<24 | uint32(lenBuf[1])<<16 | uint32(lenBuf[2])<<8 | uint32(lenBuf[3])

		typeBuf := make([]byte, 4)
		if _, err := f.Read(typeBuf); err != nil {
			return "", fmt.Errorf("read chunk type: %w", err)
		}
		chunkType := string(typeBuf)

		data := make([]byte, length)
		if _, err := f.Read(data); err != nil {
			return "", fmt.Errorf("read chunk data: %w", err)
		}

		crcBuf := make([]byte, 4)
		if _, err := f.Read(crcBuf); err != nil {
			return "", fmt.Errorf("read crc: %w", err)
		}

		if chunkType == "tEXt" {
			idx := strings.IndexByte(string(data), 0)
			if idx >= 0 {
				key := string(data[:idx])
				value := string(data[idx+1:])
				if key == "chara" {
					return value, nil
				}
			}
		}

		if chunkType == "IEND" {
			break
		}
	}

	return "", fmt.Errorf("no chara chunk found in PNG")
}

func ExtractPreviewCharaChunk(f *os.File) (string, error) {
	return extractCharaChunk(f)
}

func extractImmutable(personality string) []string {
	sentences := splitSentences(personality)

	var traits []string
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) < 5 || len(s) > 200 {
			continue
		}
		// Skip markdown/code artifacts
		if strings.HasPrefix(s, "```") || strings.HasPrefix(s, "<") {
			continue
		}
		if containsAny(strings.ToLower(s), []string{"性格", "特质", "总是", "从不", "讨厌", "喜欢", "擅长", "厌恶", "信仰", "原则"}) {
			traits = append(traits, cleanText(s))
		}
	}

	if len(traits) == 0 && personality != "" {
		for _, s := range sentences {
			s = strings.TrimSpace(s)
			// Skip markdown/code artifacts in fallback too
			if strings.HasPrefix(s, "```") || strings.HasPrefix(s, "<") {
				continue
			}
			if len(s) >= 10 && len(s) <= 200 {
				traits = append(traits, cleanText(s))
			}
			if len(traits) >= 5 {
				break
			}
		}
	}
	return traits
}

func inferTrust(scenario, personality string) int {
	text := strings.ToLower(scenario + personality)
	if containsAny(text, []string{"陌生", "初次", "不信任", "警惕", "敌对"}) {
		return 2
	}
	if containsAny(text, []string{"熟悉", "朋友", "信任", "亲密", "伙伴"}) {
		return 6
	}
	return 3
}

func inferFear(scenario, personality string) int {
	text := strings.ToLower(scenario + personality)
	if containsAny(text, []string{"逃亡", "追杀", "危险", "恐惧", "害怕"}) {
		return 7
	}
	if containsAny(text, []string{"安全", "强大", "自信", "掌控"}) {
		return 2
	}
	return 3
}

func inferStyle(mesExample, personality string) string {
	if mesExample != "" && strings.Contains(mesExample, "*") {
		return "动作描写与对话交织，带场景氛围"
	}
	return "冷淡、简洁、克制"
}

func inferRhythm(mesExample string) string {
	if mesExample == "" {
		return "短句为主"
	}
	for _, l := range strings.Split(mesExample, "\n") {
		if strings.Contains(l, "。") && len([]rune(l)) > 30 {
			return "长短交替，紧张时短促"
		}
	}
	return "短句为主，紧张时更短"
}

func inferPrimaryGoals(scenario, personality string) []GoalYAML {
	var goals []GoalYAML
	text := strings.ToLower(scenario + personality)

	if containsAny(text, []string{"逃亡", "躲藏", "活下去", "生存"}) {
		goals = append(goals, GoalYAML{ID: "survive", Priority: 10, Condition: "always"})
	}
	if containsAny(text, []string{"复仇", "追杀", "追捕", "敌人"}) {
		goals = append(goals, GoalYAML{ID: "evade_enemy", Priority: 9, Condition: "detected == true"})
	}
	if containsAny(text, []string{"任务", "使命", "目标", "完成"}) {
		goals = append(goals, GoalYAML{ID: "complete_mission", Priority: 8, Condition: "active"})
	}
	if len(goals) == 0 {
		goals = append(goals, GoalYAML{ID: "survive", Priority: 10, Condition: "always"})
	}
	return goals
}

func inferHiddenGoals(scenario, personality string) []GoalYAML {
	var goals []GoalYAML
	text := strings.ToLower(scenario + personality)

	if containsAny(text, []string{"秘密", "真相", "记忆", "过去"}) {
		goals = append(goals, GoalYAML{
			ID: "recover_memory", Priority: 8, Condition: "never",
			KnownBy: []string{}, RevealCondition: "trust > 9 AND scene == safehouse",
		})
	}
	return goals
}

func defaultForbidden() []string {
	return []string{
		"cartoon_behavior",
		"unconditional_love",
		"info_dump",
		"fourth_wall_break",
	}
}

func splitSentences(text string) []string {
	r := strings.NewReplacer(
		"。", "\n", ".", "\n",
		"！", "\n", "!", "\n",
		"？", "\n", "?", "\n",
	)
	return strings.Split(r.Replace(text), "\n")
}

func containsAny(text string, words []string) bool {
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

func cleanText(s string) string {
	s = strings.ReplaceAll(s, "{{user}}", "玩家")
	s = strings.ReplaceAll(s, "{{char}}", "角色")
	// Strip markdown code block markers that leak from v3 cards
	s = strings.ReplaceAll(s, "```markdown", "")
	s = strings.ReplaceAll(s, "```yaml", "")
	s = strings.ReplaceAll(s, "```json", "")
	s = strings.ReplaceAll(s, "```xml", "")
	s = strings.ReplaceAll(s, "```", "")
	s = strings.TrimSpace(s)
	return s
}

// extractRuleLines pulls sentences describing world rules from narrative text.
func extractRuleLines(text string) []string {
	var lines []string
	for _, s := range splitSentences(text) {
		s = strings.TrimSpace(s)
		if len(s) < 10 || len(s) > 300 {
			continue
		}
		lower := strings.ToLower(s)
		if strings.Contains(lower, "规则") || strings.Contains(lower, "系统") ||
			strings.Contains(lower, "设定") || strings.Contains(lower, "世界") ||
			strings.Contains(lower, "游戏") || strings.Contains(lower, "特色") ||
			strings.Contains(lower, "背景") {
			lines = append(lines, s)
		}
	}
	return lines
}

// inferScene builds initial SceneState from first_mes + location settings.
func inferScene(st SillyTavernChar, book []WorldBookEntry) SceneYAML {
	scene := SceneYAML{
		Location:    "未知地点",
		TimeOfDay:   "未知时间",
		Weather:     "未知天气",
		Characters:  []string{"玩家"},
		Description: "",
	}

	fm := st.FirstMes
	if fm == "" {
		return scene
	}

	// Time of day
	lower := strings.ToLower(fm)
	if containsAny(lower, []string{"太阳", "阳光", "烈日", "白天", "下午"}) {
		scene.TimeOfDay = "白天"
	} else if containsAny(lower, []string{"月亮", "夜晚", "黑夜", "深夜", "凌晨"}) {
		scene.TimeOfDay = "夜晚"
	} else if containsAny(lower, []string{"黄昏", "傍晚", "日落"}) {
		scene.TimeOfDay = "傍晚"
	}

	// Weather
	if containsAny(lower, []string{"火球", "烈日", "透蓝", "阳光", "晴朗"}) {
		scene.Weather = "晴朗炎热"
	} else if containsAny(lower, []string{"雨", "阴", "雾", "雪"}) {
		scene.Weather = "阴雨"
	}

	// Location: prefer locations mentioned in first_mes, then fall back to settings
	locs := filterWorldBookByType(book, "setting")
	locationKeywords := []string{"别墅", "酒店", "教室", "医院", "客厅", "卧室", "厨房"}
	for _, kw := range locationKeywords {
		if strings.Contains(fm, kw) {
			scene.Location = kw
			// Try to match with a setting entry for description
			for _, l := range locs {
				if strings.Contains(l.Keys, kw) || strings.Contains(l.Name, kw) {
					if scene.Description == "" {
						scene.Description = l.Content
					}
					break
				}
			}
			break
		}
	}
	if scene.Location == "未知地点" {
		for _, l := range locs {
			if strings.Contains(l.Name, "地理") {
				scene.Location = l.Keys
				if scene.Description == "" {
					scene.Description = l.Content
				}
				break
			}
		}
	}

	// Characters from first_mes
	chars := []string{"玩家"}
	if strings.Contains(fm, "赵小亮") {
		chars = append(chars, "赵小亮")
	}
	if strings.Contains(fm, "沈佳") {
		chars = append(chars, "沈佳")
	}
	if strings.Contains(fm, "阿伟") || strings.Contains(fm, "苏伟") {
		chars = append(chars, "苏伟")
	}
	if strings.Contains(fm, "梁伊伊") {
		chars = append(chars, "梁伊伊")
	}
	scene.Characters = chars

	// Description from first few narrative paragraphs
	paras := strings.Split(fm, "\n")
	var descLines []string
	for _, p := range paras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		descLines = append(descLines, cleanText(p))
		if len(descLines) >= 3 {
			break
		}
	}
	if scene.Description == "" {
		scene.Description = strings.Join(descLines, "\n")
	}

	return scene
}

func extractArc(name string) string {
	if strings.Contains(name, "主线") {
		return "主线"
	}
	if strings.Contains(name, "支线") {
		return "支线"
	}
	if strings.Contains(name, "暗线") {
		return "暗线"
	}
	if strings.Contains(name, "伏笔") {
		return "伏笔"
	}
	return ""
}

// extractCharacterBook parses v2 data.character_book.entries into WorldBookEntry.
func extractCharacterBook(data map[string]interface{}) []WorldBookEntry {
	if data == nil {
		return nil
	}
	cbRaw, ok := data["character_book"].(map[string]interface{})
	if !ok {
		return nil
	}
	entriesRaw, ok := cbRaw["entries"].([]interface{})
	if !ok {
		return nil
	}

	var result []WorldBookEntry
	for _, e := range entriesRaw {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		comment := getString(m, "comment")
		keys := getStringSlice(m, "keys")
		content := getString(m, "content")
		if content == "" {
			continue
		}

		entryType := classifyEntry(comment)
		name := comment
		if name == "" && len(keys) > 0 {
			name = keys[0]
		}

		result = append(result, WorldBookEntry{
			Type:    entryType,
			Name:    name,
			Keys:    strings.Join(keys, ", "),
			Content: cleanText(content),
		})
	}
	return result
}

func extractSystemPrompt(data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	if v, ok := data["system_prompt"].(string); ok {
		return v
	}
	return ""
}

func classifyEntry(comment string) string {
	c := strings.ToLower(comment)
	trimmed := strings.TrimSpace(comment)

	// Check bracketed category tags first: [角色], [物品], etc.
	switch {
	case strings.Contains(c, "[角色]"), strings.Contains(c, "[人物]"), strings.Contains(c, "[npc]"):
		return "character"
	case strings.Contains(c, "[事件]"), strings.Contains(c, "事件线"), strings.Contains(c, "[剧情]"):
		return "event"
	case strings.Contains(c, "[时间]"), strings.Contains(c, "时间线"), strings.Contains(c, "[年代]"):
		return "timeline"
	case strings.Contains(c, "[地点]"), strings.Contains(c, "[地理]"), strings.Contains(c, "[位置]"), strings.Contains(c, "[地图]"):
		return "location"
	case strings.Contains(c, "[物品]"), strings.Contains(c, "[装备]"), strings.Contains(c, "[道具]"), strings.Contains(c, "[武器]"):
		return "item"
	case strings.Contains(c, "[组织]"), strings.Contains(c, "[势力]"), strings.Contains(c, "[门派]"), strings.Contains(c, "[帮派]"), strings.Contains(c, "[公会]"):
		return "faction"
	case strings.Contains(c, "[体系]"), strings.Contains(c, "[能力]"), strings.Contains(c, "[职业]"), strings.Contains(c, "[技能]"), strings.Contains(c, "[等级]"), strings.Contains(c, "[系统]"):
		return "setting"
	case strings.Contains(c, "[概念]"), strings.Contains(c, "[设定]"), strings.Contains(c, "[规则]"), strings.Contains(c, "[社会]"), strings.Contains(c, "[其他]"):
		return "lore"
	}

	switch {
	case hasAnyPrefix(trimmed, "角色 -", "角色-", "人物 -", "人物-", "npc -", "npc-", "NPC -", "NPC-", "内心 -", "内心-", "角色内心 -", "角色内心-"):
		return "character"
	case hasAnyPrefix(trimmed, "事件 -", "事件-", "剧情 -", "剧情-", "章节剧情 -", "章节剧情-"):
		return "event"
	case hasAnyPrefix(trimmed, "时间 -", "时间-", "时间线 -", "时间线-", "年代 -", "年代-"):
		return "timeline"
	case hasAnyPrefix(trimmed, "地点 -", "地点-", "地理 -", "地理-", "位置 -", "位置-", "地图 -", "地图-"):
		return "location"
	case hasAnyPrefix(trimmed, "物品 -", "物品-", "装备 -", "装备-", "道具 -", "道具-", "武器 -", "武器-"):
		return "item"
	case hasAnyPrefix(trimmed, "组织 -", "组织-", "势力 -", "势力-", "门派 -", "门派-", "帮派 -", "帮派-", "公会 -", "公会-"):
		return "faction"
	case hasAnyPrefix(trimmed, "体系 -", "体系-", "能力 -", "能力-", "职业 -", "职业-", "技能 -", "技能-", "等级 -", "等级-", "系统 -", "系统-", "玩法 -", "玩法-"):
		return "setting"
	case hasAnyPrefix(trimmed, "概念 -", "概念-", "设定 -", "设定-", "规则 -", "规则-", "社会 -", "社会-", "其他 -", "其他-"):
		return "lore"
	}

	// No explicit category prefix: plain name → character entry
	if !strings.Contains(comment, "[") && !strings.Contains(comment, "]") {
		return "character"
	}

	return "lore"
}

func hasAnyPrefix(text string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func filterWorldBookByType(entries []WorldBookEntry, t string) []WorldBookEntry {
	var out []WorldBookEntry
	for _, e := range entries {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getStringSlice(m map[string]interface{}, key string) []string {
	raw, ok := m[key].([]interface{})
	if !ok {
		return nil
	}
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ImportDirectory batch imports all PNG files in a directory.
func ImportDirectory(srcDir, dstDir string) ([]string, error) {
	return ImportDirectoryWithMode(srcDir, dstDir, "auto")
}

func ImportDirectoryWithMode(srcDir, dstDir, mode string) ([]string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, err
	}

	var results []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".png" {
			continue
		}

		srcPath := filepath.Join(srcDir, entry.Name())
		charPath, worldPath, err := ImportPNGWithMode(srcPath, dstDir, mode)
		if err != nil {
			results = append(results, fmt.Sprintf("FAIL %s: %v", entry.Name(), err))
			continue
		}
		results = append(results, fmt.Sprintf("OK   %s -> char:%s world:%s", entry.Name(), charPath, worldPath))
	}

	return results, nil
}
