// Package clientip resolves the client address used for rate limiting.
package clientip

import (
	"net"
	"net/http"
	"strings"
)

// FromRequest returns the client IP. trustedProxyHops is the number of
// trusted proxies in front of the service. Zero uses the direct peer.
//
// X-Forwarded-For is only consulted when a proxy is trusted, because clients
// can set the header themselves. The selected entry is the address added by
// the outermost trusted proxy.
func FromRequest(r *http.Request, trustedProxyHops int) string {
	if trustedProxyHops > 0 {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			index := len(parts) - trustedProxyHops
			if index < 0 {
				index = 0
			}
			if address := normalize(parts[index]); address != "" {
				return address
			}
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}

	return normalize(host)
}

func normalize(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return ""
	}

	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}

	return ip.String()
}
