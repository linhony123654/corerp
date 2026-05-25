package importer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	charYAML, world := Convert(st)

	base := filepath.Base(srcPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	charPath := filepath.Join(dstDir, name+".yml")
	worldDir := filepath.Join(dstDir, "..", "worlds", name)

	charOut, _ := yaml.Marshal(charYAML)
	os.WriteFile(charPath, charOut, 0644)

	wp, err := writeWorldDir(worldDir, world)
	if err != nil {
		return "", "", fmt.Errorf("write world dir: %w", err)
	}

	return charPath, wp, nil
}

// ImportJSON reads a SillyTavern JSON card and converts to CoreRP YAMLs.
func ImportJSON(srcPath, dstDir string) (string, string, error) {
	jsonBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return "", "", fmt.Errorf("read json: %w", err)
	}

	var st SillyTavernChar
	if err := json.Unmarshal(jsonBytes, &st); err != nil {
		return "", "", fmt.Errorf("parse json: %w", err)
	}

	charYAML, world := Convert(st)

	base := filepath.Base(srcPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	charPath := filepath.Join(dstDir, name+".yml")
	worldDir := filepath.Join(dstDir, "..", "worlds", name)

	charOut, _ := yaml.Marshal(charYAML)
	os.WriteFile(charPath, charOut, 0644)

	wp, err := writeWorldDir(worldDir, world)
	if err != nil {
		return "", "", fmt.Errorf("write world dir: %w", err)
	}

	return charPath, wp, nil
}

// writeWorldDir creates the three-layer world directory structure.
func writeWorldDir(dir string, w WorldYAML) (string, error) {
	canonDir := filepath.Join(dir, "canon")
	os.MkdirAll(canonDir, 0755)

	// 1. world.yml — core_rules only
	worldFile := filepath.Join(dir, "world.yml")
	worldData, _ := yaml.Marshal(map[string]interface{}{
		"name":       w.Name,
		"core_rules": w.CoreRules,
	})
	os.WriteFile(worldFile, worldData, 0644)

	// 2. scene.yml — initial scene
	sceneFile := filepath.Join(dir, "scene.yml")
	sceneData, _ := yaml.Marshal(w.Scene)
	os.WriteFile(sceneFile, sceneData, 0644)

	// 3. canon/ontology.yml — entity definitions
	ontoFile := filepath.Join(canonDir, "ontology.yml")
	ontoData, _ := yaml.Marshal(w.Ontology)
	os.WriteFile(ontoFile, ontoData, 0644)

	// 4. canon/facts.yml — immutable facts extracted from settings + lore
	factsFile := filepath.Join(canonDir, "facts.yml")
	facts := extractFacts(w.Ontology)
	factsData, _ := yaml.Marshal(map[string]interface{}{"facts": facts})
	os.WriteFile(factsFile, factsData, 0644)

	return worldFile, nil
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
	return charYAML, worldYAML
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

	// No bracket prefix: plain name → character entry
	if !strings.Contains(comment, "[") && !strings.Contains(comment, "]") {
		return "character"
	}

	return "lore"
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
		charPath, worldPath, err := ImportPNG(srcPath, dstDir)
		if err != nil {
			results = append(results, fmt.Sprintf("FAIL %s: %v", entry.Name(), err))
			continue
		}
		results = append(results, fmt.Sprintf("OK   %s -> char:%s world:%s", entry.Name(), charPath, worldPath))
	}

	return results, nil
}
