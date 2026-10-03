package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// RealIP replaces r.RemoteAddr with the client address that a trusted reverse
// proxy reports in X-Real-IP or X-Forwarded-For. Headers from any other peer
// are ignored, so a client that connects directly cannot choose the IP that is
// logged and stored with its session.
func RealIP(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if peer, ok := remoteIP(r.RemoteAddr); ok && isTrusted(peer, trustedProxies) {
				if client, ok := forwardedClientIP(r.Header); ok {
					r.RemoteAddr = client.String()
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func remoteIP(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func isTrusted(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// forwardedClientIP prefers X-Real-IP, which Traefik sets to the address it
// accepted the connection from, and otherwise takes the last X-Forwarded-For
// entry: the one the trusted proxy appended, not one the client supplied.
func forwardedClientIP(header http.Header) (netip.Addr, bool) {
	if addr, err := netip.ParseAddr(strings.TrimSpace(header.Get("X-Real-IP"))); err == nil {
		return addr.Unmap(), true
	}
	forwarded := header.Values("X-Forwarded-For")
	if len(forwarded) == 0 {
		return netip.Addr{}, false
	}
	entries := strings.Split(forwarded[len(forwarded)-1], ",")
	if addr, err := netip.ParseAddr(strings.TrimSpace(entries[len(entries)-1])); err == nil {
		return addr.Unmap(), true
	}
	return netip.Addr{}, false
}
