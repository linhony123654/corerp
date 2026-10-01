package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRPCompositionRecordedSpeechToneVersionAndAttribution(t *testing.T) {
	for tone, phrase := range map[string]string{"gentle": "温和地", "firm": "坚定地", "teasing": "打趣地", "hesitant": "语调迟疑地", "flat": "语调平淡地"} {
		t.Run(tone, func(t *testing.T) {
			in := naturalFixture()
			in.Facts[1].SpeechTone = tone
			plan, err := BuildDefaultRPComposition(in)
			if err != nil {
				t.Fatal(err)
			}
			view, err := RenderRPComposition(context.Background(), in, plan, nil)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Version != RPFactCompositionVersionV3 || view.CompositionVersion != plan.Version || view.Artifact.Plan.Version != plan.Version {
				t.Fatalf("version mismatch: %+v", view)
			}
			if !strings.Contains(view.Lines[1], "贾母"+phrase+"应道") || strings.Contains(view.Lines[0], phrase) {
				t.Fatalf("tone attribution: %v", view.Lines)
			}
			if view.Artifact.Input.Facts[1].SpeechTone != tone {
				t.Fatal("artifact lost source tone")
			}
			schema := RPCompositionSchema(in)
			if schema["properties"].(map[string]any)["version"].(map[string]any)["const"] != plan.Version {
				t.Fatal("schema version mismatch")
			}
			raw, _ := json.Marshal(plan)
			if _, err := DecodeRPCompositionPlan(raw, in); err != nil {
				t.Fatal(err)
			}
			plan.Version = RPFactCompositionVersionV2
			if _, err := RenderRPComposition(context.Background(), in, plan, nil); err == nil {
				t.Fatal("v2 accepted sourced tone")
			}
		})
	}
	in := naturalFixture()
	plan, _ := BuildDefaultRPComposition(in)
	view, err := RenderRPComposition(context.Background(), in, plan, nil)
	if err != nil || view.CompositionVersion != RPFactCompositionVersionV2 || view.Lines[1] != "「宝玉来了。有什么话，慢慢说给老祖宗听。」贾母应道，露出笑容。" {
		t.Fatalf("v2 changed: %+v %v", view, err)
	}
	plan.Version = RPFactCompositionVersionV3
	if _, err := RenderRPComposition(context.Background(), in, plan, nil); err == nil {
		t.Fatal("v3 accepted without tone")
	}
}

func TestRPCompositionRejectsUnsourcedOrNonSpeechTone(t *testing.T) {
	for _, tc := range []struct {
		index int
		tone  string
	}{{1, "whisper"}, {2, "gentle"}, {0, "private_happy"}} {
		in := naturalFixture()
		in.Facts[tc.index].SpeechTone = tc.tone
		if _, err := BuildDefaultRPComposition(in); !HasCode(err, CodeProjectionDiverged) {
			t.Fatalf("accepted invalid tone %+v: %v", tc, err)
		}
	}
	in := naturalFixture()
	in.PublicPresentations[0].Text = "语气温和，古典措辞。"
	plan, err := BuildDefaultRPComposition(in)
	if err != nil {
		t.Fatal(err)
	}
	view, err := RenderRPComposition(context.Background(), in, plan, nil)
	if err != nil || plan.Version != RPFactCompositionVersionV2 || strings.Contains(strings.Join(view.Lines, ""), "温和") {
		t.Fatalf("inferred tone: %+v %v", view, err)
	}
}

func TestRPCompositionFiniteExistingNonverbalCodes(t *testing.T) {
	for code, word := range map[string]string{"look_at": "看", "smile": "笑", "nod": "头", "shake_head": "头", "turn_away": "身", "frown": "眉", "wave": "挥", "shrug": "肩", "raise_hand": "手", "beckon": "招"} {
		t.Run(code, func(t *testing.T) {
			in := naturalFixture()
			in.PublicPresentations = nil
			in.Facts = []RPNarrativeFact{{EventID: "action", ActorID: "player", ActorName: "宝玉", Action: "expression", ExpressionCode: code, TargetActorID: "grandmother", TargetActorName: "贾母"}}
			plan, err := BuildDefaultRPComposition(in)
			if err != nil {
				t.Fatal(err)
			}
			view, err := RenderRPComposition(context.Background(), in, plan, nil)
			if err != nil || len(view.Lines) != 1 || !strings.Contains(view.Lines[0], word) || !strings.Contains(view.Lines[0], "贾母") || plan.Version != RPFactCompositionVersionV2 {
				t.Fatalf("missing grounded gesture: %+v %v", view, err)
			}
		})
	}
	in := naturalFixture()
	in.Facts[2].ExpressionCode = "embrace"
	if _, err := BuildDefaultRPComposition(in); err == nil {
		t.Fatal("unsupported expression accepted")
	}
}
