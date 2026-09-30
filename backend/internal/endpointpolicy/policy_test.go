package endpointpolicy

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fixedResolver struct {
	answers []net.IPAddr
	calls   atomic.Int32
}

func (r *fixedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	r.calls.Add(1)
	return append([]net.IPAddr(nil), r.answers...), nil
}

type blockingResolver struct{}

func (blockingResolver) LookupIPAddr(ctx context.Context, _ string) ([]net.IPAddr, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestPrepareDNSLookupRespectsProviderTimeout(t *testing.T) {
	policy := Policy{AllowedOrigins: []string{"https://models.example.test"}, Resolver: blockingResolver{}}
	start := time.Now()
	if _, _, err := policy.PrepareChatCompletions("https://models.example.test/v1/chat/completions", 20*time.Millisecond); err == nil {
		t.Fatal("DNS lookup ignored provider timeout")
	}
	if time.Since(start) > time.Second {
		t.Fatal("DNS lookup exceeded provider timeout budget")
	}
}

func TestPrepareCanonicalizesAndEnforcesOriginAllowlist(t *testing.T) {
	resolver := &fixedResolver{answers: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}}
	policy := Policy{AllowedOrigins: []string{"https://api.example.test"}, Resolver: resolver}
	canonical, _, err := policy.PrepareChatCompletions("HTTPS://API.EXAMPLE.TEST:443/v1/chat/completions", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if canonical != "https://api.example.test/v1/chat/completions" {
		t.Fatalf("non-canonical URL %q", canonical)
	}
	if _, _, err := policy.Prepare("https://other.example.test/v1/chat/completions", time.Second); err == nil {
		t.Fatal("non-allowlisted origin was accepted")
	}
	for _, raw := range []string{
		"https://user:secret@api.example.test/v1/chat/completions",
		"https://api.example.test/v1/chat/completions?token=secret",
		"https://api.example.test/v1/chat/completions#fragment",
		"http://api.example.test/v1/chat/completions",
		"https://api.example.test/v1/../v1/chat/completions",
		"https://api.example.test/v1/%2e%2e/admin",
		"https://api.example.test/v1%2fadmin/chat/completions",
		"https://api.example.test\\@other.example.test/path",
		"https://api.example.test/admin/export",
	} {
		if _, _, err := policy.Prepare(raw, time.Second); err == nil {
			t.Errorf("unsafe URL accepted: %q", raw)
		}
	}
	if _, _, err := policy.PrepareModels("https://api.example.test/v1/chat/completions", time.Second); err == nil {
		t.Fatal("models route accepted a chat-completions path")
	}
	if _, _, err := policy.PrepareChatCompletions("https://api.example.test/v1/models", time.Second); err == nil {
		t.Fatal("chat route accepted a models path")
	}
}

func TestEnvironmentAllowlistDerivesOnlyProviderAPIRoutes(t *testing.T) {
	policy, err := FromEnvironment("https://api.example.test/v1/chat/completions", "")
	if err != nil {
		t.Fatal(err)
	}
	policy.Resolver = &fixedResolver{answers: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}}
	if _, _, err := policy.PrepareChatCompletions("https://api.example.test/v1/chat/completions", time.Second); err != nil {
		t.Fatalf("operator chat endpoint denied: %v", err)
	}
	if _, _, err := policy.PrepareModels("https://api.example.test/v1/models", time.Second); err != nil {
		t.Fatalf("associated model-list endpoint denied: %v", err)
	}
	for _, target := range []string{"https://api.example.test/admin/export", "https://api.example.test/v1/admin"} {
		if _, _, err := policy.Prepare(target, time.Second); err == nil {
			t.Fatalf("same-origin non-provider path accepted: %s", target)
		}
	}
}

func TestPrepareBlocksPrivateLoopbackLinkLocalMetadataAndSpecialUseDNS(t *testing.T) {
	for _, address := range []string{
		"127.0.0.1", "10.0.0.5", "169.254.1.2", "169.254.169.254", "::1", "fc00::5", "fe80::1",
		"100.64.0.1", "100.100.100.200", "::ffff:100.100.100.200", "198.18.0.1", "::ffff:198.18.0.1",
		"192.0.2.1", "240.0.0.1", "2001:db8::1", "64:ff9b:1::1",
	} {
		t.Run(address, func(t *testing.T) {
			resolver := &fixedResolver{answers: []net.IPAddr{{IP: net.ParseIP(address)}}}
			policy := Policy{AllowedOrigins: []string{"https://models.example.test"}, Resolver: resolver}
			if _, _, err := policy.Prepare("https://models.example.test/v1/chat/completions", time.Second); err == nil {
				t.Fatalf("blocked DNS answer %s was accepted", address)
			}
		})
	}
}

func TestExactLocalAllowlistPermitsPrivateButNeverMetadata(t *testing.T) {
	local := Policy{
		LocalOrigins: []string{"http://model.internal.test:11434"},
		Resolver:     &fixedResolver{answers: []net.IPAddr{{IP: net.ParseIP("10.0.0.7")}}},
	}
	canonical, _, err := local.Prepare("http://MODEL.INTERNAL.test:11434/v1/chat/completions", time.Second)
	if err != nil || canonical != "http://model.internal.test:11434/v1/chat/completions" {
		t.Fatalf("exact local operator allowlist rejected private model: %q %v", canonical, err)
	}
	local.LocalOrigins = []string{"http://metadata.internal.test"}
	for _, address := range []string{"169.254.169.254", "100.100.100.200", "100.64.0.1", "198.18.0.1"} {
		local.Resolver = &fixedResolver{answers: []net.IPAddr{{IP: net.ParseIP(address)}}}
		if _, _, err := local.PrepareChatCompletions("http://metadata.internal.test/v1/chat/completions", time.Second); err == nil {
			t.Errorf("metadata/special-use address %s was accepted through the local allowlist", address)
		}
	}
	local.LocalOrigins = []string{"http://lan.internal.test"}
	for _, address := range []string{"169.254.170.2", "fe80::1"} {
		local.Resolver = &fixedResolver{answers: []net.IPAddr{{IP: net.ParseIP(address)}}}
		if _, _, err := local.PrepareChatCompletions("http://lan.internal.test/v1/chat/completions", time.Second); err == nil {
			t.Fatalf("link-local address %s was accepted through the local allowlist", address)
		}
	}
}

func TestPreparedClientPinsCheckedDNSAndDoesNotFollowRedirects(t *testing.T) {
	var requests atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("local fixture"))
	}))
	defer destination.Close()
	parsed := strings.TrimPrefix(destination.URL, "http://")
	_, port, err := net.SplitHostPort(parsed)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &fixedResolver{answers: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}
	policy := Policy{
		LocalOrigins: []string{"http://model.fixture:" + port},
		Resolver:     resolver,
	}
	endpoint, client, err := policy.PrepareModels("http://model.fixture:"+port+"/v1/models", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if requests.Load() != 1 || resolver.calls.Load() != 1 {
		t.Fatalf("request did not use the single validated DNS result: requests=%d lookups=%d", requests.Load(), resolver.calls.Load())
	}

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer redirect.Close()
	redirectHost, redirectPort, err := net.SplitHostPort(strings.TrimPrefix(redirect.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	redirectPolicy := Policy{
		LocalOrigins: []string{"http://redirect.fixture:" + redirectPort},
		Resolver:     &fixedResolver{answers: []net.IPAddr{{IP: net.ParseIP(redirectHost)}}},
	}
	redirectURL, redirectClient, err := redirectPolicy.PrepareModels("http://redirect.fixture:"+redirectPort+"/v1/models", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	redirectResponse, err := redirectClient.Get(redirectURL)
	if err != nil {
		t.Fatal(err)
	}
	redirectResponse.Body.Close()
	if requests.Load() != 1 || redirectResponse.StatusCode != http.StatusFound {
		t.Fatalf("redirect was followed: destination requests=%d status=%d", requests.Load(), redirectResponse.StatusCode)
	}
}

func TestTestLocalhostPolicyIsExplicitAndProductionDefaultDeniesLocal(t *testing.T) {
	if _, _, err := (Policy{}).Prepare("http://127.0.0.1:1234/v1/models", time.Second); err == nil {
		t.Fatal("production default permitted localhost")
	}
	if _, _, err := TestLocalhostPolicy().Prepare("http://127.0.0.1:1234/v1/models", time.Second); err != nil {
		t.Fatalf("explicit fixture policy rejected loopback: %v", err)
	}
}
