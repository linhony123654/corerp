// Package endpointpolicy validates operator-authorized provider destinations and
// pins each HTTP transport to the DNS answers that were checked for the request.
package endpointpolicy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type routeKind uint8

const (
	anyAPIRoute routeKind = iota
	chatCompletionsRoute
	modelsRoute
)

// Policy authorizes exact origins and the associated OpenAI-compatible API
// paths. LocalOrigins is a separate operator allowlist for private/LAN model
// servers. TestLocalhost is only intended for httptest fixtures and is never
// enabled by production defaults.
type Policy struct {
	AllowedOrigins []string
	LocalOrigins   []string
	AllowedPaths   map[string][]string
	LocalPaths     map[string][]string
	Resolver       IPResolver
	TestLocalhost  bool
}

type IPResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// TestLocalhostPolicy opts a test fixture into loopback access and only the
// standard chat-completions/models routes. Production code must use exact
// operator allowlists instead.
func TestLocalhostPolicy() Policy { return Policy{TestLocalhost: true} }

// FromEnvironment parses comma-separated operator-authorized endpoint URLs.
// A root origin allows only the conventional chat-completions/models routes;
// an endpoint path establishes only its corresponding API route family.
func FromEnvironment(allowed, local string) (Policy, error) {
	policy := Policy{}
	for _, raw := range splitList(allowed) {
		if err := policy.addEndpoint(raw, false); err != nil {
			return Policy{}, errors.New("invalid provider endpoint allowlist")
		}
	}
	for _, raw := range splitList(local) {
		if err := policy.addEndpoint(raw, true); err != nil {
			return Policy{}, errors.New("invalid local provider endpoint allowlist")
		}
	}
	return policy, nil
}

// WithOperatorEndpoint adds the API route family implied by an operator-owned
// provider endpoint. It does not grant private-address access; local endpoints
// must also be explicitly listed in LocalOrigins.
func (p Policy) WithOperatorEndpoint(raw string) Policy {
	_ = p.addEndpoint(raw, false)
	return p
}

// WithLocalOperatorEndpoint adds exactly the local origin and its chat/models
// route family. Call only for an endpoint explicitly configured by the operator.
func (p Policy) WithLocalOperatorEndpoint(raw string) Policy {
	u, origin, err := canonicalURL(raw)
	if err == nil && isLocalName(u.Hostname()) {
		p.addEndpointAt(u, origin, true)
	}
	return p
}

// Canonicalize validates URL syntax and canonicalizes scheme, authority and
// path without granting access or performing DNS lookup.
func Canonicalize(raw string) (string, error) {
	u, _, err := canonicalURL(raw)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// Prepare validates an OpenAI-compatible chat/models URL. Provider code should
// use PrepareChatCompletions or PrepareModels to constrain its intended route.
func (p Policy) Prepare(raw string, timeout time.Duration) (string, *http.Client, error) {
	return p.prepare(raw, timeout, anyAPIRoute)
}

func (p Policy) PrepareChatCompletions(raw string, timeout time.Duration) (string, *http.Client, error) {
	return p.prepare(raw, timeout, chatCompletionsRoute)
}

func (p Policy) PrepareModels(raw string, timeout time.Duration) (string, *http.Client, error) {
	return p.prepare(raw, timeout, modelsRoute)
}

func (p Policy) prepare(raw string, timeout time.Duration, route routeKind) (string, *http.Client, error) {
	u, origin, err := canonicalURL(raw)
	if err != nil {
		return "", nil, err
	}
	host := u.Hostname()
	localAllowed := contains(p.LocalOrigins, origin)
	testLocalAllowed := p.TestLocalhost && isLoopbackName(host)
	if u.Scheme == "http" && !localAllowed && !testLocalAllowed {
		return "", nil, errors.New("remote provider endpoints require HTTPS")
	}
	if !localAllowed && !testLocalAllowed && !contains(p.AllowedOrigins, origin) {
		return "", nil, errors.New("provider endpoint is not allowlisted")
	}
	if !p.pathAllowed(origin, u.Path, localAllowed, testLocalAllowed, route) {
		return "", nil, errors.New("provider endpoint path is not allowlisted")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	lookupCtx, cancelLookup := context.WithTimeout(context.Background(), timeout)
	defer cancelLookup()
	ips, err := p.resolve(lookupCtx, host, localAllowed || testLocalAllowed)
	if err != nil {
		return "", nil, err
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var lastErr error
			for _, ip := range ips {
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				lastErr = dialErr
			}
			if lastErr == nil {
				lastErr = errors.New("no approved provider address")
			}
			return nil, lastErr
		},
	}
	client := &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return u.String(), client, nil
}

func (p Policy) pathAllowed(origin, targetPath string, local, testLocal bool, route routeKind) bool {
	paths := p.AllowedPaths[origin]
	if local {
		paths = p.LocalPaths[origin]
	}
	if len(paths) == 0 {
		if testLocal || local || contains(p.AllowedOrigins, origin) {
			paths = defaultAPIPaths()
		} else {
			return false
		}
	}
	chat, models := false, false
	for _, allowed := range paths {
		if targetPath == allowed {
			if strings.HasSuffix(allowed, "/chat/completions") {
				chat = true
			}
			if strings.HasSuffix(allowed, "/models") {
				models = true
			}
		}
	}
	switch route {
	case chatCompletionsRoute:
		return chat
	case modelsRoute:
		return models
	default:
		return chat || models
	}
}

func (p *Policy) addEndpoint(raw string, local bool) error {
	u, origin, err := canonicalURL(raw)
	if err != nil {
		return err
	}
	p.addEndpointAt(u, origin, local)
	return nil
}

func (p *Policy) addEndpointAt(u *url.URL, origin string, local bool) {
	if local {
		p.LocalOrigins = appendUnique(p.LocalOrigins, origin)
		p.LocalPaths = addPaths(p.LocalPaths, origin, routePaths(u.Path))
		return
	}
	p.AllowedOrigins = appendUnique(p.AllowedOrigins, origin)
	p.AllowedPaths = addPaths(p.AllowedPaths, origin, routePaths(u.Path))
}

func addPaths(existing map[string][]string, origin string, paths []string) map[string][]string {
	if existing == nil {
		existing = make(map[string][]string)
	}
	merged := append([]string(nil), existing[origin]...)
	for _, item := range paths {
		merged = appendUnique(merged, item)
	}
	existing[origin] = merged
	return existing
}

func routePaths(configuredPath string) []string {
	p := strings.TrimSuffix(configuredPath, "/")
	if p == "" {
		return defaultAPIPaths()
	}
	addPair := func(base string) []string {
		return []string{base + "/chat/completions", base + "/models"}
	}
	for _, suffix := range []string{"/chat/completions", "/completions", "/models"} {
		if strings.HasSuffix(p, suffix) {
			base := strings.TrimSuffix(p, suffix)
			paths := addPair(base)
			if strings.HasSuffix(base, "/v1") {
				paths = append(paths, addPair(strings.TrimSuffix(base, "/v1"))...)
			}
			return paths
		}
	}
	if p == "/v1" || strings.HasSuffix(p, "/v1") {
		paths := addPair(p)
		paths = append(paths, addPair(strings.TrimSuffix(p, "/v1"))...)
		return paths
	}
	if idx := strings.Index(p, "/v1/"); idx >= 0 {
		base := p[:idx]
		paths := addPair(base + "/v1")
		paths = append(paths, addPair(base)...)
		return paths
	}
	return addPair(p + "/v1")
}

func defaultAPIPaths() []string {
	return []string{"/v1/chat/completions", "/v1/models", "/chat/completions", "/models"}
}

func (p Policy) AllowsLocal(raw string) bool {
	u, origin, err := canonicalURL(raw)
	if err != nil {
		return false
	}
	return contains(p.LocalOrigins, origin) || p.TestLocalhost && isLoopbackName(u.Hostname())
}

func (p Policy) resolve(ctx context.Context, host string, allowPrivate bool) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !safeAddress(ip, allowPrivate) {
			return nil, errors.New("provider endpoint resolves to a blocked address")
		}
		return []net.IP{ip}, nil
	}
	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	answers, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(answers) == 0 {
		return nil, errors.New("provider endpoint DNS lookup failed")
	}
	ips := make([]net.IP, 0, len(answers))
	seen := make(map[string]bool, len(answers))
	for _, answer := range answers {
		ip := answer.IP
		if !safeAddress(ip, allowPrivate) {
			return nil, errors.New("provider endpoint resolves to a blocked address")
		}
		key := ip.String()
		if !seen[key] {
			seen[key] = true
			ips = append(ips, ip)
		}
	}
	return ips, nil
}

func safeAddress(ip net.IP, allowPrivate bool) bool {
	if ip == nil {
		return false
	}
	// Normalize IPv4-mapped IPv6 answers so neither the special-use checks nor
	// private-address exception can be bypassed by an alternate representation.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	} else if v6 := ip.To16(); v6 != nil {
		ip = v6
	} else {
		return false
	}
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || isMetadataAddress(ip) || isSpecialUseAddress(ip) {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || !ip.IsGlobalUnicast() {
		return allowPrivate && (ip.IsLoopback() || ip.IsPrivate())
	}
	return true
}

func isMetadataAddress(ip net.IP) bool {
	for _, raw := range []string{
		"169.254.169.0/24",   // Common cloud instance metadata endpoints.
		"168.63.129.16/32",   // Azure platform virtual IP.
		"100.100.100.200/32", // Alibaba Cloud instance metadata.
		"fd00:ec2::254/128",  // AWS IPv6 instance metadata.
	} {
		_, block, _ := net.ParseCIDR(raw)
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// isSpecialUseAddress rejects special-purpose ranges that Go's
// IP.IsGlobalUnicast may still classify as globally-unicast. They must not be
// reachable through provider credentials, even with a local endpoint allowlist.
func isSpecialUseAddress(ip net.IP) bool {
	var ranges []string
	if ip.To4() != nil {
		ranges = []string{
			"0.0.0.0/8",       // This network.
			"100.64.0.0/10",   // Shared address space / CGNAT.
			"192.0.0.0/24",    // IETF protocol assignments.
			"192.0.2.0/24",    // Documentation.
			"192.88.99.0/24",  // Deprecated 6to4 relay anycast.
			"198.18.0.0/15",   // Benchmarking.
			"198.51.100.0/24", // Documentation.
			"203.0.113.0/24",  // Documentation.
			"240.0.0.0/4",     // Reserved for future use.
		}
	} else {
		ranges = []string{
			"64:ff9b::/96",   // Well-known NAT64; may encode a blocked IPv4 target.
			"64:ff9b:1::/48", // Local-use NAT64.
			"100::/64",       // Discard-only.
			"2001::/23",      // IETF protocol assignments and transition mechanisms.
			"2001:db8::/32",  // Documentation.
			"2002::/16",      // 6to4 transition prefix.
			"3fff::/20",      // Documentation.
		}
	}
	for _, raw := range ranges {
		_, block, _ := net.ParseCIDR(raw)
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

func canonicalURL(raw string) (*url.URL, string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || strings.ContainsAny(text, "\\\r\n\t") {
		return nil, "", errors.New("invalid provider endpoint")
	}
	u, err := url.Parse(text)
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || strings.Contains(u.EscapedPath(), "%") {
		return nil, "", errors.New("invalid provider endpoint")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", errors.New("invalid provider endpoint")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || strings.Contains(host, "%") {
		return nil, "", errors.New("invalid provider endpoint")
	}
	ip := net.ParseIP(host)
	if ip != nil {
		host = ip.String()
	} else if !validHostname(host) {
		return nil, "", errors.New("invalid provider endpoint")
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, "", errors.New("invalid provider endpoint")
		}
		port = strconv.Itoa(n)
	}
	if port == "80" && u.Scheme == "http" || port == "443" && u.Scheme == "https" {
		port = ""
	}
	if port == "" {
		if strings.Contains(host, ":") {
			u.Host = "[" + host + "]"
		} else {
			u.Host = host
		}
	} else {
		u.Host = net.JoinHostPort(host, port)
	}
	if u.Path == "" {
		u.Path = "/"
	}
	cleanPath := path.Clean("/" + strings.TrimPrefix(u.Path, "/"))
	if strings.HasSuffix(u.Path, "/") && u.Path != "/" {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	if cleanPath != u.Path || strings.Contains(u.Path, "//") {
		return nil, "", errors.New("invalid provider endpoint path")
	}
	u.RawPath = ""
	origin := u.Scheme + "://" + u.Host
	return u, origin, nil
}

func canonicalOrigin(raw string) (string, string, error) {
	u, origin, err := canonicalURL(strings.TrimSpace(raw))
	if err != nil {
		return "", "", errors.New("invalid provider origin")
	}
	return origin, u.Hostname(), nil
}

func splitList(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func appendUnique(values []string, value string) []string {
	if !contains(values, value) {
		return append(values, value)
	}
	return values
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func isLoopbackName(host string) bool {
	ip := net.ParseIP(host)
	return strings.EqualFold(host, "localhost") || ip != nil && ip.IsLoopback()
}

func isLocalName(host string) bool {
	ip := net.ParseIP(host)
	return isLoopbackName(host) || ip != nil && ip.IsPrivate()
}

func validHostname(host string) bool {
	if len(host) > 253 || host == "" {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
