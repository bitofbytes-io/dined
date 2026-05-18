package auth

import "testing"

func TestGoogleAuthenticatorEmailAllowlist(t *testing.T) {
	authenticator := &GoogleAuthenticator{
		allowedDomains: map[string]struct{}{"example.org": {}},
		allowedEmails:  map[string]struct{}{"family@example.com": {}},
	}

	tests := []struct {
		email string
		want  bool
	}{
		{email: "Family@Example.com", want: true},
		{email: "person@example.org", want: true},
		{email: "person@gmail.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := authenticator.IsEmailAllowed(tt.email); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
