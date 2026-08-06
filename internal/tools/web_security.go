package tools

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const maxWebResponseBytes = 5 * 1024 * 1024

// SanitizeUntrustedText removes invisible formatting/private-use characters
// commonly used to conceal prompt-injection instructions in remote content.
func SanitizeUntrustedText(value string) string {
	current := value
	for iteration := 0; iteration < 10; iteration++ {
		previous := current
		current = norm.NFKC.String(current)
		current = strings.Map(func(r rune) rune {
			if unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Co, r) || unicode.Is(unicode.Categories["Cn"], r) {
				return -1
			}
			return r
		}, current)
		if current == previous {
			break
		}
	}
	return current
}

func validatePublicHTTPSURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("URL %q must use https", rawURL)
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("URL %q must include a hostname", rawURL)
	}
	if parsed.User != nil {
		return fmt.Errorf("URL %q must not include credentials", rawURL)
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") {
		return fmt.Errorf("URL %q targets a local hostname", rawURL)
	}
	if address := net.ParseIP(hostname); address != nil && !isPublicIP(address) {
		return fmt.Errorf("URL %q targets a non-public address", rawURL)
	}
	return nil
}

func isPublicIP(address net.IP) bool {
	return address != nil &&
		!address.IsLoopback() &&
		!address.IsPrivate() &&
		!address.IsLinkLocalUnicast() &&
		!address.IsLinkLocalMulticast() &&
		!address.IsUnspecified() &&
		!address.IsMulticast()
}

func newSecureWebClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("invalid destination %q: %w", address, err)
			}
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve %q: %w", host, err)
			}
			if len(addresses) == 0 {
				return nil, fmt.Errorf("hostname %q resolved to no addresses", host)
			}
			for _, resolved := range addresses {
				if !isPublicIP(resolved.IP) {
					return nil, fmt.Errorf("hostname %q resolves to non-public address %s", host, resolved.IP)
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
		},
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return validatePublicHTTPSURL(request.URL.String())
		},
	}
}
