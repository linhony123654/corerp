package core

import "testing"

func TestRPBackgroundRejectsUnboundedOrInconsistentRoutine(t *testing.T) {
	valid := RPBackgroundRequest{PrincipalID: "creator", InstanceID: "world", BranchID: "main", EntityID: "person", ExpectedHead: 1, IdempotencyKey: "key", AgeMin: 20, AgeMax: 30, ResidencePlaceID: "home", InitialPlaceID: "cafe", Schedule: []RPBackgroundSchedule{{WorldTime: "2026-09-22T03:00:00Z", PlaceID: "home", ActivityCode: "home"}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*RPBackgroundRequest){
		func(r *RPBackgroundRequest) { r.AgeMin = 31 },
		func(r *RPBackgroundRequest) { r.AgeMax = 151 },
		func(r *RPBackgroundRequest) { r.Schedule = nil },
		func(r *RPBackgroundRequest) { r.InitialPlaceID = "home" },
		func(r *RPBackgroundRequest) {
			r.Schedule = append(r.Schedule, RPBackgroundSchedule{WorldTime: "2026-09-22T03:00:00Z", PlaceID: "cafe", ActivityCode: "present"})
		},
	} {
		r := valid
		change(&r)
		if err := r.Validate(); err == nil {
			t.Fatal("accepted invalid background")
		}
	}
}
