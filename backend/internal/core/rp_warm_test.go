package core

import "testing"

func TestRPWarmDecisionUsesOnlyOwnUpcomingWorkAndLegalRoute(t *testing.T) {
	makeInput := func() RPWarmDecisionInput {
		return RPWarmDecisionInput{ActorID: "ada", WorldTime: "2026-09-23T07:00:00Z", PlaceID: "cafe", ActivityCode: "lunch", NextSchedule: &RPDecisionSchedule{SourceEventID: "own-contract", WorldTime: "2026-09-23T08:00:00Z", PlaceID: "work", ActivityCode: "work"}, ReachablePlaceIDs: []string{"home", "work"}}
	}
	in := makeInput()
	got, err := ProposeRPWarmDecision(in)
	if err != nil || got.Action != "leave" || got.ToPlaceID != "work" || got.ScheduleSourceEventID != "own-contract" {
		t.Fatalf("own preparation: %+v %v", got, err)
	}
	for name, change := range map[string]func(*RPWarmDecisionInput){
		"no-appointment":          func(i *RPWarmDecisionInput) { i.NextSchedule = nil },
		"ordinary-appointment":    func(i *RPWarmDecisionInput) { i.NextSchedule.ActivityCode = "lunch" },
		"already-working":         func(i *RPWarmDecisionInput) { i.ActivityCode = "work" },
		"already-there":           func(i *RPWarmDecisionInput) { i.PlaceID = "work" },
		"too-early":               func(i *RPWarmDecisionInput) { i.WorldTime = "2026-09-23T06:59:59Z" },
		"due-owned-by-scheduler":  func(i *RPWarmDecisionInput) { i.WorldTime = "2026-09-23T08:00:00Z" },
		"past-owned-by-scheduler": func(i *RPWarmDecisionInput) { i.WorldTime = "2026-09-23T09:00:00Z" },
		"closed-or-unknown-route": func(i *RPWarmDecisionInput) { i.ReachablePlaceIDs = []string{"home"} },
	} {
		t.Run(name, func(t *testing.T) {
			in := makeInput()
			change(&in)
			got, err := ProposeRPWarmDecision(in)
			if err != nil || got.Action != "wait" || got.ToPlaceID != "" {
				t.Fatalf("invented action: %+v %v", got, err)
			}
		})
	}
	for name, change := range map[string]func(*RPWarmDecisionInput){
		"missing-source": func(i *RPWarmDecisionInput) { i.NextSchedule.SourceEventID = "" },
		"invalid-clock":  func(i *RPWarmDecisionInput) { i.WorldTime = "not-time" },
		"invalid-due":    func(i *RPWarmDecisionInput) { i.NextSchedule.WorldTime = "not-time" },
		"unproven-delay": func(i *RPWarmDecisionInput) { i.NextSchedule.OriginalWorldTime = "2026-09-23T06:00:00Z" },
	} {
		t.Run(name, func(t *testing.T) {
			in := makeInput()
			change(&in)
			if _, err := ProposeRPWarmDecision(in); !HasCode(err, CodeInvalidArgument) {
				t.Fatalf("accepted invalid context: %v", err)
			}
		})
	}
	in = makeInput()
	in.PlaceID = "home"
	in.ReachablePlaceIDs = []string{"cafe"}
	in.WorkPath = []string{"home", "cafe", "work"}
	in.RouteSourceEventIDs = []string{"route-one", "route-two"}
	got, err = ProposeRPWarmDecision(in)
	if err != nil || got.ToPlaceID != "cafe" || got.Action != "leave" {
		t.Fatalf("sourced waypoint: %+v %v", got, err)
	}
	in.RouteSourceEventIDs[1] = ""
	if _, err := ProposeRPWarmDecision(in); !HasCode(err, CodeInvalidArgument) {
		t.Fatal("accepted route without provenance")
	}
	in = makeInput()
	in.NextSchedule.OriginalWorldTime = "2026-09-23T06:00:00Z"
	in.NextSchedule.DelaySourceEventID = "actual-road-delay"
	got, err = ProposeRPWarmDecision(in)
	if err != nil || got.Action != "leave" || got.DelaySourceEventID != "actual-road-delay" {
		t.Fatalf("lost delay provenance: %+v %v", got, err)
	}
}
