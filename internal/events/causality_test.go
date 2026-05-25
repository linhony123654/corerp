package events

import "testing"

func TestIsMeaninglessContent(t *testing.T) {
	tests := []struct {
		name  string
		in    interface{}
		want  bool
	}{
		{name: "empty string", in: "", want: true},
		{name: "quoted empty double", in: "\"\"", want: true},
		{name: "quoted empty single", in: "''", want: true},
		{name: "whitespace only", in: "   \n\t", want: true},
		{name: "normal text", in: "anya", want: false},
		{name: "number", in: 111, want: false},
		{name: "nil", in: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMeaninglessContent(tt.in); got != tt.want {
				t.Fatalf("isMeaninglessContent(%v)=%v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
