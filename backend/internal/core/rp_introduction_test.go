package core

import "testing"

func TestExplicitSelfIntroductionRequiresSpokenCanonicalName(t *testing.T) {
	for _, sample := range []struct {
		text, name string
		want       bool
	}{
		{"我叫 Ada。", "Ada", true},
		{"我是管家蔡，账已理清。", "蔡", true},
		{"My name is Ada.", "Ada", true},
		{"你来了。", "蔡", false},
		{"我是管家。", "蔡", false},
		{"我是 Ada 的朋友。", "Ada", false},
		{"听说 Ada 来了，我是管家。", "Ada", false},
	} {
		if got := ExplicitSelfIntroduction(sample.text, sample.name); got != sample.want {
			t.Errorf("%q with %q: got %v, want %v", sample.text, sample.name, got, sample.want)
		}
	}
}
