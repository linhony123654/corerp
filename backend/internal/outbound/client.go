package outbound

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func ValidateEndpoint(raw string, allowLoopback bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("invalid endpoint")
	}
	if strings.EqualFold(u.Hostname(), "localhost") && !allowLoopback {
		return errors.New("local endpoint is not allowed")
	}
	ip := net.ParseIP(u.Hostname())
	if ip != nil && blockedIP(ip, allowLoopback) {
		return errors.New("private endpoint is not allowed")
	}
	if u.Scheme == "http" && !(allowLoopback && (strings.EqualFold(u.Hostname(), "localhost") || ip != nil && ip.IsLoopback())) {
		return errors.New("remote endpoint requires HTTPS")
	}
	return nil
}

func NewClient(timeout time.Duration, allowLoopback bool) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("endpoint DNS resolution failed")
		}
		for _, ip := range ips {
			if blockedIP(ip, allowLoopback) {
				return nil, errors.New("endpoint resolved to a blocked address")
			}
		}
		var last error
		for _, ip := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func blockedIP(ip net.IP, allowLoopback bool) bool {
	if allowLoopback && ip.IsLoopback() {
		return false
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || !ip.IsGlobalUnicast()
}
