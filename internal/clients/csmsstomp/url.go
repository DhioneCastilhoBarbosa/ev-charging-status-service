package csmsstomp

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseAPIHost extrai host e TLS a partir da base REST da Move
// (ex.: https://cs-test.use-move.com/api/v1).
func ParseAPIHost(intelbrasAPIBase string) (host string, useTLS bool, err error) {
	u, err := url.Parse(strings.TrimSpace(intelbrasAPIBase))
	if err != nil || u.Host == "" {
		return "", false, fmt.Errorf("invalid INTELBRAS_BASE_URL: %q", intelbrasAPIBase)
	}
	switch u.Scheme {
	case "https", "wss":
		useTLS = true
	case "http", "ws", "":
		useTLS = false
	default:
		return "", false, fmt.Errorf("unsupported URL scheme in INTELBRAS_BASE_URL: %s", u.Scheme)
	}
	return u.Host, useTLS, nil
}
