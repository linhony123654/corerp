package importer

import (
	"testing"
)

func TestClassifyEntryBracketed(t *testing.T) {
	tests := []struct {
		comment string
		want    string
	}{
		{"[角色] 主角", "character"},
		{"[人物] 配角", "character"},
		{"[NPC] 路人", "character"},
		{"[地点] 夜之城", "location"},
		{"[地理] 荒原", "location"},
		{"[物品] 武器", "item"},
		{"[装备] 护甲", "item"},
		{"[组织] 公司", "faction"},
		{"[势力] 帮派", "faction"},
		{"[体系] 能力系统", "setting"},
		{"[职业] 黑客", "setting"},
		{"[概念] 规则", "lore"},
		{"[设定] 世界观", "lore"},
		{"[事件] 初次相遇", "event"},
		{"[剧情] 主线", "event"},
		{"[时间] 2077年", "timeline"},
		{"[年代] 中年", "timeline"},
	}
	for _, tc := range tests {
		got := classifyEntry(tc.comment)
		if got != tc.want {
			t.Errorf("classifyEntry(%q) = %s, want %s", tc.comment, got, tc.want)
		}
	}
}

func TestClassifyEntryPlainName(t *testing.T) {
	// Plain names without brackets default to character
	if got := classifyEntry("张三"); got != "character" {
		t.Errorf("plain name should be character, got %s", got)
	}
}

func TestClassifyEntryPlainPrefixes(t *testing.T) {
	tests := []struct {
		comment string
		want    string
	}{
		{"角色 - 贾宝玉", "character"},
		{"内心 - 贾宝玉-内心世界", "character"},
		{"地点 - 会芳园", "location"},
		{"道具 - 花笺", "item"},
		{"组织 - 四王八公", "faction"},
		{"章节剧情 - 第8章", "event"},
		{"玩法 - 女儿酒令", "setting"},
	}
	for _, tc := range tests {
		if got := classifyEntry(tc.comment); got != tc.want {
			t.Fatalf("classifyEntry(%q) = %s, want %s", tc.comment, got, tc.want)
		}
	}
}

func TestExtractImmutable(t *testing.T) {
	// Empty input
	traits := extractImmutable("")
	if len(traits) != 0 {
		t.Errorf("expected 0 traits for empty input, got %d", len(traits))
	}
}

func TestExtractImmutableWithTraits(t *testing.T) {
	personality := "性格冷淡、沉默寡言。从不在人前表露情感。擅长黑客技术。讨厌公司。"
	traits := extractImmutable(personality)
	if len(traits) == 0 {
		t.Error("expected traits but got none")
	}
}

func TestInferTrust(t *testing.T) {
	tests := []struct {
		scenario    string
		personality string
		want        int
	}{
		{"陌生环境中的初次见面", "警惕", 2},
		{"和熟悉的朋友一起冒险", "信任", 6},
		{"", "", 3},
	}
	for _, tc := range tests {
		got := inferTrust(tc.scenario, tc.personality)
		if got != tc.want {
			t.Errorf("inferTrust(%q, %q) = %d, want %d", tc.scenario, tc.personality, got, tc.want)
		}
	}
}

func TestInferFear(t *testing.T) {
	tests := []struct {
		scenario    string
		personality string
		want        int
	}{
		{"被追杀逃亡中", "恐惧害怕", 7},
		{"安全的环境中", "自信强大掌控", 2},
		{"", "", 3},
	}
	for _, tc := range tests {
		got := inferFear(tc.scenario, tc.personality)
		if got != tc.want {
			t.Errorf("inferFear(%q, %q) = %d, want %d", tc.scenario, tc.personality, got, tc.want)
		}
	}
}

func TestDefaultForbidden(t *testing.T) {
	fb := defaultForbidden()
	if len(fb) == 0 {
		t.Error("default forbidden should not be empty")
	}
	expected := []string{"cartoon_behavior", "unconditional_love", "info_dump", "fourth_wall_break"}
	for _, exp := range expected {
		found := false
		for _, f := range fb {
			if f == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing forbidden action: %s", exp)
		}
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"{{user}} says hello", "玩家 says hello"},
		{"{{char}} responds", "角色 responds"},
		{"```markdown\nsome text\n```", "some text"},
		{"```yaml\nkey: value\n```", "key: value"},
	}
	for _, tc := range tests {
		got := cleanText(tc.input)
		if got != tc.want {
			t.Errorf("cleanText(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestConvertV1(t *testing.T) {
	st := SillyTavernChar{
		Name:        "测试角色",
		Description: "一个测试用的角色描述。",
		Personality: "冷静、理性、不轻易表露情感。",
		Scenario:    "在一个安全的实验室里。",
		FirstMes:    "你好，我是测试角色。请坐。",
		MesExample:  "*推了推眼镜* 你需要什么帮助？",
	}
	char, world := Convert(st)
	if char.Identity.Name != "测试角色" {
		t.Errorf("name = %s, want 测试角色", char.Identity.Name)
	}
	if len(char.Identity.Immutable) == 0 {
		t.Error("expected immutable traits, got none")
	}
	if char.Identity.Adaptive == nil {
		t.Error("expected adaptive stats, got nil")
	}
	if len(char.Identity.Forbidden) == 0 {
		t.Error("expected forbidden list, got empty")
	}
	if world.Name != "测试角色" {
		t.Errorf("world name = %s, want 测试角色", world.Name)
	}
}

func TestConvertV2(t *testing.T) {
	st := SillyTavernChar{
		Spec:        "chara_card_v2",
		Name:        "old_name",
		Description: "old_desc",
		Data: map[string]interface{}{
			"name":        "新名字",
			"description": "新描述",
			"personality": "乐观开朗、热情奔放。",
			"scenario":    "在热闹的集市中。",
		},
	}
	char, _ := Convert(st)
	if char.Identity.Name != "新名字" {
		t.Errorf("v2 name should be 新名字, got %s", char.Identity.Name)
	}
}

func TestBuildWorldYAMLOntology(t *testing.T) {
	st := SillyTavernChar{
		Name:     "测试",
		FirstMes: "在一个晴朗的下午，你走进了咖啡馆。",
	}
	book := []WorldBookEntry{
		{Type: "character", Name: "NPC1", Keys: "npc", Content: "一个友好的NPC"},
		{Type: "faction", Name: "公司A", Keys: "公司", Content: "一家大型科技公司"},
		{Type: "location", Name: "咖啡馆", Keys: "coffee", Content: "一个温馨的咖啡馆"},
		{Type: "setting", Name: "能力体系", Keys: "能力", Content: "三种能力等级"},
		{Type: "lore", Name: "[设定] 世界观", Keys: "世界观", Content: "世界设定内容"},
		{Type: "event", Name: "主线事件", Keys: "主线", Content: "关键情节"},
		{Type: "item", Name: "关键物品", Keys: "物品", Content: "一把重要的钥匙"},
	}
	world := BuildWorldYAML(st, book)

	if len(world.Ontology.Characters) != 1 {
		t.Errorf("expected 1 character, got %d", len(world.Ontology.Characters))
	}
	if len(world.Ontology.Factions) != 1 {
		t.Errorf("expected 1 faction, got %d", len(world.Ontology.Factions))
	}
	if len(world.Ontology.Locations) != 1 {
		t.Errorf("expected 1 location, got %d", len(world.Ontology.Locations))
	}
	if len(world.Ontology.Settings) != 1 {
		t.Errorf("expected 1 setting, got %d", len(world.Ontology.Settings))
	}
	if len(world.Ontology.Lore) != 1 {
		t.Errorf("expected 1 lore, got %d", len(world.Ontology.Lore))
	}
	if len(world.Ontology.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(world.Ontology.Events))
	}
	if len(world.Ontology.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(world.Ontology.Items))
	}
	// Scene inferred from first_mes — "下午" maps to 白天
	if world.Scene.TimeOfDay != "白天" {
		t.Errorf("scene time = %s, want 白天", world.Scene.TimeOfDay)
	}
	// "咖啡馆" is not in the fixed locationKeywords list, so defaults to 未知地点
	if world.Scene.Location == "" {
		t.Error("scene location should not be empty")
	}
}

func TestDetectCardKindEnsemble(t *testing.T) {
	st := SillyTavernChar{Name: "红楼梦"}
	book := []WorldBookEntry{
		{Type: "character", Name: "贾宝玉", Content: "性格敏感，身份复杂，关系众多。"},
		{Type: "character", Name: "林黛玉", Content: "性格孤高，目标隐晦，关系复杂。"},
		{Type: "character", Name: "薛宝钗", Content: "性格稳重，身份清晰，关系繁多。"},
		{Type: "character", Name: "王熙凤", Content: "精明强势，掌控欲明显。"},
	}
	if got := detectCardKind(st, book); got != CardKindEnsemble {
		t.Fatalf("detectCardKind = %s, want %s", got, CardKindEnsemble)
	}
}

func TestConvertBundleEnsembleGeneratesMultipleCharacters(t *testing.T) {
	st := SillyTavernChar{
		Name:        "红楼梦",
		Description: "大观园群像故事。",
		FirstMes:    "贾宝玉抬眼看向林黛玉，薛宝钗在一旁静静地坐着。",
		Data: map[string]interface{}{
			"character_book": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"comment": "[角色] 贾宝玉", "keys": []interface{}{"贾宝玉"}, "content": "性格敏感，身份复杂，喜欢诗意与自由。"},
					map[string]interface{}{"comment": "[角色] 林黛玉", "keys": []interface{}{"林黛玉"}, "content": "性格孤高，情绪细腻，关系紧张。"},
					map[string]interface{}{"comment": "[角色] 薛宝钗", "keys": []interface{}{"薛宝钗"}, "content": "性格稳重，处事圆融，目标克制。"},
					map[string]interface{}{"comment": "[角色] 王熙凤", "keys": []interface{}{"王熙凤"}, "content": "精明强势，善于掌控局面。"},
				},
			},
		},
	}

	bundle := ConvertBundle(st)
	if bundle.Kind != CardKindEnsemble {
		t.Fatalf("bundle kind = %s, want ensemble", bundle.Kind)
	}
	if len(bundle.Characters) < 3 {
		t.Fatalf("generated characters = %d, want >= 3", len(bundle.Characters))
	}
	if bundle.PrimaryCharacter == "" {
		t.Fatal("primary character should not be empty")
	}
	if _, ok := bundle.Characters[bundle.PrimaryCharacter]; !ok {
		t.Fatalf("primary character %q missing from generated set", bundle.PrimaryCharacter)
	}
	if len(bundle.CastIndex.GeneratedAtImport) == 0 {
		t.Fatal("cast index should include generated cast")
	}
}

func TestNormalizeEntryNameStripsBulletPrefix(t *testing.T) {
	got := normalizeEntryName("- 贾宝玉", "")
	if got != "贾宝玉" {
		t.Fatalf("normalizeEntryName = %q, want 贾宝玉", got)
	}
}

func TestNormalizeEntryNameStripsInnerWorldWrapper(t *testing.T) {
	got := normalizeEntryName("内心 - 彩云-内心世界", "")
	if got != "彩云" {
		t.Fatalf("normalizeEntryName = %q, want 彩云", got)
	}
}

func TestNormalizeEntryNameStripsInnerWorldTopicSuffix(t *testing.T) {
	got := normalizeEntryName("内心 - 贾宝玉-对黛玉的深情", "")
	if got != "贾宝玉" {
		t.Fatalf("normalizeEntryName = %q, want 贾宝玉", got)
	}
}

func TestRankCastCandidatesPrefersOpeningCast(t *testing.T) {
	st := SillyTavernChar{
		Name:        "红楼梦",
		Description: "贾宝玉与林黛玉、薛宝钗的群像故事。",
		FirstMes:    "贾宝玉看向林黛玉，薛宝钗在一旁静静地坐着。",
	}
	book := []WorldBookEntry{
		{Type: "character", Name: "- 乌进孝", Content: "性格朴实，身份是庄头，负责进贡。"},
		{Type: "character", Name: "- 贾宝玉", Content: "性格敏感，身份复杂，关系众多，喜欢诗意与自由。"},
		{Type: "character", Name: "- 林黛玉", Content: "性格孤高，情绪细腻，关系紧张。"},
		{Type: "character", Name: "- 薛宝钗", Content: "性格稳重，处事圆融，目标克制。"},
	}
	ranked := rankCastCandidates(st, book)
	if len(ranked) == 0 {
		t.Fatal("ranked candidates should not be empty")
	}
	if ranked[0].Name != "贾宝玉" {
		t.Fatalf("top candidate = %q, want 贾宝玉", ranked[0].Name)
	}
}

func TestRankCastCandidatesMergesAliasesIntoCanonicalName(t *testing.T) {
	st := SillyTavernChar{
		Name: "红楼梦",
	}
	book := []WorldBookEntry{
		{Type: "character", Name: "角色 - 宝玉", Keys: "宝玉, 贾宝玉, 宝二爷", Content: "性格敏感，身份复杂，喜欢诗意与自由。"},
		{Type: "character", Name: "角色 - 贾宝玉", Keys: "贾宝玉, 宝玉, 怡红公子", Content: "与林黛玉关系深，目标是守住真情。"},
		{Type: "character", Name: "角色 - 乌进孝", Keys: "乌进孝, 乌庄头", Content: "庄头，负责进贡。"},
	}

	ranked := rankCastCandidates(st, book)
	if len(ranked) == 0 {
		t.Fatal("ranked candidates should not be empty")
	}
	if ranked[0].Name != "贾宝玉" {
		t.Fatalf("top candidate = %q, want 贾宝玉", ranked[0].Name)
	}
	if ranked[0].Score <= ranked[1].Score {
		t.Fatalf("expected merged 贾宝玉 score to exceed 乌进孝: top=%d second=%d", ranked[0].Score, ranked[1].Score)
	}
}

func TestChooseCanonicalNamePrefersFormalFullName(t *testing.T) {
	got := chooseCanonicalName("宝玉", []string{"宝玉", "贾宝玉", "怡红公子"})
	if got != "贾宝玉" {
		t.Fatalf("chooseCanonicalName = %q, want 贾宝玉", got)
	}
}
