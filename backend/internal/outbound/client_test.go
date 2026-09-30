package outbound

import "testing"

func TestValidateEndpointRejectsInternalTargets(t *testing.T) {
	for _, endpoint := range []string{
		"http://127.0.0.1:8080/v1",
		"https://10.0.0.1/v1",
		"https://192.168.1.1/v1",
		"https://169.254.169.254/latest/meta-data",
		"https://100.64.0.1/v1",
		"https://[::1]/v1",
	} {
		if err := ValidateEndpoint(endpoint, false); err == nil {
			t.Fatalf("internal endpoint accepted: %s", endpoint)
		}
	}
	if err := ValidateEndpoint("https://api.example.com/v1/chat/completions", false); err != nil {
		t.Fatal("public HTTPS endpoint rejected", err)
	}
	if err := ValidateEndpoint("http://127.0.0.1:8080/v1", true); err != nil {
		t.Fatal("explicit test/operator loopback rejected", err)
	}
}
