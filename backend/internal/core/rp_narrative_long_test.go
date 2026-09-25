package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestRPNarrativeLongDensityKeepsAttributedFactsAndDoesNotPadSparseScene(t *testing.T) {
	style := DefaultRPStyle()
	style.NarrativeDensity = "long"
	input := RPNarrativeInput{ControlledEntityID: "lin", Style: style, Facts: []RPNarrativeFact{}}
	for i := 0; i < 7; i++ {
		input.Facts = append(input.Facts, RPNarrativeFact{EventID: fmt.Sprintf("event-%d", i), ActorID: "lin", ActorName: "Lin", Action: "speak", Text: fmt.Sprintf("第%d段：", i) + strings.Repeat("今天我想把这件事讲清楚，", 11), WorldTime: "2026-09-25T09:00:00Z", PlaceName: "咖啡馆"})
	}
	chunks := []RPNarrativeChunk{}
	view, err := (DeterministicRPNarrativeProvider{}).RenderStream(context.Background(), input, func(chunk RPNarrativeChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil || len(view.Lines) != 7 || len(chunks) != 7 || len([]rune(strings.Join(view.Lines, "\n"))) < 1000 {
		t.Fatalf("long source-rich fixture was truncated or padded incorrectly: lines=%d chars=%d err=%v", len(view.Lines), len([]rune(strings.Join(view.Lines, "\n"))), err)
	}
	for i, fact := range input.Facts {
		if view.EventIDs[i] != fact.EventID || chunks[i].EventID != fact.EventID || chunks[i].Index != i || chunks[i].Line != view.Lines[i] || !strings.Contains(view.Lines[i], "「"+fact.Text+"」") || !strings.Contains(view.Lines[i], fact.WorldTime) || !strings.Contains(view.Lines[i], fact.PlaceName) {
			t.Fatalf("long narrative fact %d lost attribution or exact accepted text: %+v", i, chunks[i])
		}
	}
	sparse := input
	sparse.Facts = []RPNarrativeFact{{EventID: "event-sparse", ActorID: "lin", ActorName: "Lin", Action: "speak", Text: "你好", PlaceName: "咖啡馆"}}
	short, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), sparse)
	if err != nil || len([]rune(strings.Join(short.Lines, ""))) >= 1000 || !strings.Contains(short.Lines[0], "「你好」") {
		t.Fatalf("long mode invented a minimum-length scene: %+v %v", short, err)
	}
	legacy := sparse
	legacy.Style = DefaultRPStyle()
	old, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), legacy)
	if err != nil || len(old.Lines) != 1 || old.Lines[0] != "你说：「你好」" {
		t.Fatalf("missing legacy density changed old output: %+v %v", old, err)
	}
	standard := sparse
	standard.Style = DefaultRPStyle()
	standard.Style.NarrativeDensity = "standard"
	standard.Style.DescriptionDensity = 80
	standardView, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), standard)
	if err != nil || !strings.Contains(standardView.Lines[0], "在咖啡馆") {
		t.Fatalf("standard density lost sourced place: %+v %v", standardView, err)
	}
	conciseScene := standard
	conciseScene.Style.NarrativeDensity = "concise"
	conciseView, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), conciseScene)
	if err != nil || strings.Contains(conciseView.Lines[0], "在咖啡馆") || conciseView.Lines[0] != "你说：「你好」" {
		t.Fatalf("concise density changed accepted quote or added setting: %+v %v", conciseView, err)
	}
}
