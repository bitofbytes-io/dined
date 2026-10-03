package auth

import "testing"

func TestAllowlistAllowed(t *testing.T) {
	allowlist := NewAllowlist([]string{"family@example.com"}, []string{"example.org"})

	tests := []struct {
		email string
		want  bool
	}{
		{email: "Family@Example.com", want: true},
		{email: "person@example.org", want: true},
		{email: "person@gmail.com", want: false},
		{email: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := allowlist.Allowed(tt.email); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllowlistAllowedClaimsRequiresHostedDomainForDomainEntries(t *testing.T) {
	allowlist := NewAllowlist([]string{"family@example.com"}, []string{"Example.org"})

	tests := []struct {
		name   string
		claims *GoogleClaims
		want   bool
	}{
		{name: "listed email without hd", claims: &GoogleClaims{Email: "family@example.com"}, want: true},
		{name: "domain email with matching hd", claims: &GoogleClaims{Email: "person@example.org", HostedDomain: "example.org"}, want: true},
		{name: "domain email with hd in other case", claims: &GoogleClaims{Email: "Person@Example.org", HostedDomain: "EXAMPLE.ORG"}, want: true},
		{name: "domain email without hd", claims: &GoogleClaims{Email: "person@example.org"}, want: false},
		{name: "domain email with other hd", claims: &GoogleClaims{Email: "person@example.org", HostedDomain: "evil.example"}, want: false},
		{name: "unlisted email", claims: &GoogleClaims{Email: "person@gmail.com"}, want: false},
		{name: "nil claims", claims: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowlist.AllowedClaims(tt.claims); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
