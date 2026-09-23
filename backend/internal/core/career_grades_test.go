package core

import "testing"

func TestCareerGradeScaleOrderAndBoundaries(t *testing.T) {
	s := CareerGradeScale{OrganizationID: "org", Grades: []string{"z-entry", "a-lead", "middle-director"}}
	for _, tc := range []struct{ from, to, want string }{
		{"z-entry", "a-lead", "promotion"}, {"middle-director", "a-lead", "demotion"}, {"a-lead", "a-lead", "transfer"},
	} {
		got, err := s.Transition(tc.from, tc.to)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: %s %v", tc, got, err)
		}
	}
	for _, grades := range [][]string{nil, {"one"}, {"one", "one"}, {"one", " "}, make([]string, 17)} {
		bad := CareerGradeScale{OrganizationID: "org", Grades: grades}
		if !HasCode(bad.Validate(), CodeInvalidArgument) {
			t.Fatal("invalid scale accepted")
		}
	}
	for _, pair := range [][2]string{{"unknown", "a-lead"}, {"a-lead", "unknown"}} {
		if _, err := s.Transition(pair[0], pair[1]); !HasCode(err, CodeInvalidArgument) {
			t.Fatal("unknown grade inferred")
		}
	}
	r := CareerGradeScaleRequest{Scale: s}
	if !HasCode(r.Validate(), CodeInvalidArgument) {
		t.Fatal("missing binding accepted")
	}
}
