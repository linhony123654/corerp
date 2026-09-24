package core

import "testing"

func TestRPCultureEvaluationOwnKnownNorms(t *testing.T) {
	c := RPCulture{CultureID: "giving", VersionEventID: "definition_1", ScopeKind: "community", ScopeID: "group_1", GroupIdentity: "mutual aid", Norms: []RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: 2}}}
	own := RPCultureInternalization{EntityID: "ada", CultureID: c.CultureID, VersionEventID: c.VersionEventID, TransmissionEventID: "heard_1", StanceEventID: "stance_1", Stance: "accept"}
	for _, tc := range []struct {
		stance string
		want   int
	}{{"accept", 2}, {"partial", 1}, {"oppose", 0}, {"rebel", -2}} {
		t.Run(tc.stance, func(t *testing.T) {
			stance := own
			stance.Stance = tc.stance
			got, err := EvaluateRPCulture(c, stance, "ada", "gift")
			if err != nil || len(got) != 1 || got[0].Score != tc.want || got[0].Basis != stance {
				t.Fatalf("evaluation=%+v error=%v", got, err)
			}
		})
	}
	c.Norms[0].Evaluation = -2
	got, err := EvaluateRPCulture(c, own, "ada", "gift")
	if err != nil || len(got) != 1 || got[0].Score != -2 {
		t.Fatalf("opposing group: %+v %v", got, err)
	}
	got, err = EvaluateRPCulture(c, own, "ada", "greet")
	if err != nil || len(got) != 0 {
		t.Fatalf("unregulated action: %+v %v", got, err)
	}
	for _, change := range []func(*RPCultureInternalization){
		func(s *RPCultureInternalization) { s.EntityID = "bo" },
		func(s *RPCultureInternalization) { s.VersionEventID = "unseen_new_version" },
		func(s *RPCultureInternalization) { s.CultureID = "other" },
		func(s *RPCultureInternalization) { s.TransmissionEventID = "" },
		func(s *RPCultureInternalization) { s.StanceEventID = "" },
		func(s *RPCultureInternalization) { s.Stance = "automatic" },
	} {
		invalid := own
		change(&invalid)
		if _, err := EvaluateRPCulture(c, invalid, "ada", "gift"); !HasCode(err, CodeInvalidArgument) {
			t.Fatalf("accepted invalid stance %+v: %v", invalid, err)
		}
	}
}

func TestRPCultureRejectsUnsupportedNorm(t *testing.T) {
	c := RPCulture{CultureID: "c", VersionEventID: "v", ScopeKind: "family", ScopeID: "f", GroupIdentity: "family", Norms: []RPCultureNorm{{NormID: "n", Action: "bypass_authorization", Evaluation: 2}}}
	if err := c.Validate(); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("unsupported norm: %v", err)
	}
	c.Norms[0].Action = "gift"
	c.Norms = append(c.Norms, c.Norms[0])
	if err := c.Validate(); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("duplicate norm: %v", err)
	}
}
