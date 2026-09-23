package core

import "testing"

func TestCareerAnnouncementInput(t *testing.T) {
	r := CareerAnnouncementRequest{Binding: CareerBinding{PrincipalID: "manager", InstanceID: "world", BranchID: "main", ExpectedHead: 1, IdempotencyKey: "news"}, AnnouncementID: "news", ContractID: "job", SpeakerID: "speaker"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CareerAnnouncementRequest){func(r *CareerAnnouncementRequest) { r.SpeakerID = "" }, func(r *CareerAnnouncementRequest) { r.ContractID = "" }, func(r *CareerAnnouncementRequest) { r.AnnouncementID = "" }, func(r *CareerAnnouncementRequest) { r.Binding.PrincipalID = "" }} {
		bad := r
		mutate(&bad)
		if !HasCode(bad.Validate(), CodeInvalidArgument) {
			t.Fatal("invalid announcement accepted")
		}
	}
}
