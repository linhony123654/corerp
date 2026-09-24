package core

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRPNarrativeStreamOrderingCancellationAndIncompleteFacts(t *testing.T) {
	p := DeterministicRPNarrativeProvider{}
	in := RPNarrativeInput{Style: DefaultRPStyle(), Facts: []RPNarrativeFact{
		{EventID: "one", ActorName: "Lin", Action: "speak", Text: "你好\n继续。"},
		{EventID: "two", ActorName: "Cai", Action: "respond", Text: "你好。"},
	}}
	want, err := p.Render(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	var chunks []RPNarrativeChunk
	got, err := p.RenderStream(context.Background(), in, func(c RPNarrativeChunk) error { chunks = append(chunks, c); return nil })
	if err != nil || !reflect.DeepEqual(got, want) || len(chunks) != 2 || chunks[0].Index != 0 || chunks[1].Index != 1 || chunks[0].Line != want.Lines[0] || chunks[1].EventID != "two" {
		t.Fatalf("stream differs from sync view: %+v %v", chunks, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	count := 0
	_, err = p.RenderStream(ctx, in, func(c RPNarrativeChunk) error { count++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || count != 1 {
		t.Fatalf("cancel did not stop between facts: %d %v", count, err)
	}
	broken := errors.New("client disconnected")
	count = 0
	_, err = p.RenderStream(context.Background(), in, func(c RPNarrativeChunk) error { count++; return broken })
	if !errors.Is(err, broken) || count != 1 {
		t.Fatalf("write failure ignored: %d %v", count, err)
	}
	// First line is genuinely emitted before a later render fails; the caller
	// must require its explicit transport completion marker.
	in.Facts[1].Action = "invented"
	count = 0
	_, err = p.RenderStream(context.Background(), in, func(c RPNarrativeChunk) error { count++; return nil })
	if !HasCode(err, CodeProjectionDiverged) || count != 1 {
		t.Fatalf("not incrementally rendered: %d %v", count, err)
	}
}
