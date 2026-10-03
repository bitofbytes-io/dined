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
				if client, ok := forwardedClientIP(r.Header, trustedProxies); ok {
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
// accepted the connection from. Otherwise it walks X-Forwarded-For from the
// right, skipping trusted proxy hops, and returns the first other address:
// entries further left were supplied by the client and cannot be trusted.
func forwardedClientIP(header http.Header, trustedProxies []netip.Prefix) (netip.Addr, bool) {
	if addr, err := netip.ParseAddr(strings.TrimSpace(header.Get("X-Real-IP"))); err == nil {
		return addr.Unmap(), true
	}
	entries := strings.Split(strings.Join(header.Values("X-Forwarded-For"), ","), ",")
	for i := len(entries) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(entries[i]))
		if err != nil {
			return netip.Addr{}, false
		}
		if addr = addr.Unmap(); !isTrusted(addr, trustedProxies) {
			return addr, true
		}
	}
	return netip.Addr{}, false
}
