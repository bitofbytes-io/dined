package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestRealIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name       string
		remoteAddr string
		realIP     string
		forwarded  []string
		want       string
	}{
		{name: "trusted proxy X-Real-IP", remoteAddr: "10.0.1.5:51234", realIP: "203.0.113.7", forwarded: []string{"198.51.100.1"}, want: "203.0.113.7"},
		{name: "trusted proxy last X-Forwarded-For entry", remoteAddr: "10.0.1.5:51234", forwarded: []string{"198.51.100.1, 203.0.113.7"}, want: "203.0.113.7"},
		{name: "trusted proxy last X-Forwarded-For header", remoteAddr: "10.0.1.5:51234", forwarded: []string{"198.51.100.1", "203.0.113.7"}, want: "203.0.113.7"},
		{name: "trusted proxy without headers", remoteAddr: "10.0.1.5:51234", want: "10.0.1.5:51234"},
		{name: "trusted proxy invalid header", remoteAddr: "10.0.1.5:51234", realIP: "not-an-ip", want: "10.0.1.5:51234"},
		{name: "untrusted peer X-Real-IP ignored", remoteAddr: "198.51.100.9:443", realIP: "203.0.113.7", want: "198.51.100.9:443"},
		{name: "untrusted peer X-Forwarded-For ignored", remoteAddr: "198.51.100.9:443", forwarded: []string{"203.0.113.7"}, want: "198.51.100.9:443"},
		{name: "IPv4-mapped trusted proxy", remoteAddr: "[::ffff:10.0.1.5]:51234", realIP: "203.0.113.7", want: "203.0.113.7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}
			for _, value := range tt.forwarded {
				req.Header.Add("X-Forwarded-For", value)
			}
			var got string
			RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.RemoteAddr
			})).ServeHTTP(httptest.NewRecorder(), req)
			if got != tt.want {
				t.Fatalf("RemoteAddr = %q, want %q", got, tt.want)
			}
		})
	}
}
