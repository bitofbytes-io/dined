package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/middleware"
)

func TestSafeRedirectPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/log", want: true},
		{path: "/log?q=a+b%20c&next=https%3A%2F%2Fexample.com#photos", want: true},
		{path: "/places/100%25", want: true},
		{path: `/\evil.example/log`, want: false},
		{path: "/%5cevil.example/log", want: false},
		{path: "/%2fevil.example/log", want: false},
		{path: "/log?x=%5c", want: false},
		{path: "/log%0aevil", want: false},
		{path: "/log?x=%0d%0aLocation:evil", want: false},
		{path: "/log\t", want: false},
		{path: "/log%7f", want: false},
		{path: "javascript:alert(1)", want: false},
		{path: "/invalid%xx", want: false},
		{path: "log", want: false},
		{path: "/log?restaurant=Pizza", want: true},
		{path: "", want: false},
		{path: "https://evil.example/log", want: false},
		{path: "//evil.example/log", want: false},
		{path: "/%2f%2fevil.example/log", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isSafeRedirectPath(tt.path); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGoogleLoginPathRejectsExternalRedirect(t *testing.T) {
	if got := googleLoginPath("https://evil.example"); got != "/api/auth/google" {
		t.Fatalf("got %q", got)
	}
}

type fakeGoogleAuthenticator struct {
	claims        *auth.GoogleClaims
	allowed       bool
	exchangeCalls int
}

func (f *fakeGoogleAuthenticator) AuthURL(state string) string {
	return "https://accounts.example/auth?state=" + url.QueryEscape(state)
}

func (f *fakeGoogleAuthenticator) Exchange(context.Context, string) (*auth.GoogleClaims, error) {
	f.exchangeCalls++
	return f.claims, nil
}

func (f *fakeGoogleAuthenticator) IsAllowed(*auth.GoogleClaims) bool {
	return f.allowed
}

func encodedOAuthState(t *testing.T, payload oauthStatePayload) string {
	t.Helper()
	stateJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(stateJSON)
}

func TestGoogleCallback(t *testing.T) {
	verified := &auth.GoogleClaims{Sub: "google-1", Email: "family@example.com", EmailVerified: true, Name: "Family"}
	unverified := &auth.GoogleClaims{Sub: "google-2", Email: "family@example.com", EmailVerified: false}
	tests := []struct {
		name          string
		cookieState   string
		paramState    string
		claims        *auth.GoogleClaims
		allowed       bool
		wantLocation  string
		wantExchanges int
		wantSession   bool
	}{
		{
			name:         "state mismatch",
			cookieState:  "cookie-state",
			paramState:   "other-state",
			claims:       verified,
			allowed:      true,
			wantLocation: "/login?message=" + url.QueryEscape("Invalid login state. Please try again."),
		},
		{
			name:          "unverified email",
			cookieState:   "state",
			paramState:    "state",
			claims:        unverified,
			allowed:       true,
			wantLocation:  "/login?message=" + url.QueryEscape("Please verify your Google email address before logging in."),
			wantExchanges: 1,
		},
		{
			name:          "disallowed email",
			cookieState:   "state",
			paramState:    "state",
			claims:        verified,
			allowed:       false,
			wantLocation:  "/login?message=" + url.QueryEscape("That Google account is not allowed to access Dined."),
			wantExchanges: 1,
		},
		{
			name:          "allowed verified email",
			cookieState:   "state",
			paramState:    "state",
			claims:        verified,
			allowed:       true,
			wantLocation:  "/log",
			wantExchanges: 1,
			wantSession:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			googleAuth := &fakeGoogleAuthenticator{claims: tt.claims, allowed: tt.allowed}
			authService := auth.NewService(auth.NewMemoryRepository(), time.Hour, nil)
			h := New(&config.Config{AuthSessionTTL: time.Hour}, nil, nil, authService, googleAuth)
			query := url.Values{
				"state": {encodedOAuthState(t, oauthStatePayload{State: tt.paramState, Redirect: "/log"})},
				"code":  {"auth-code"},
			}
			req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?"+query.Encode(), nil)
			req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: tt.cookieState})
			rec := httptest.NewRecorder()

			h.GoogleCallback(rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
			}
			if got := rec.Header().Get("Location"); got != tt.wantLocation {
				t.Fatalf("location = %q, want %q", got, tt.wantLocation)
			}
			if googleAuth.exchangeCalls != tt.wantExchanges {
				t.Fatalf("exchange calls = %d, want %d", googleAuth.exchangeCalls, tt.wantExchanges)
			}
			var sessionToken string
			for _, cookie := range rec.Result().Cookies() {
				if cookie.Name == middleware.CookieName && cookie.MaxAge > 0 {
					sessionToken = cookie.Value
				}
			}
			if (sessionToken != "") != tt.wantSession {
				t.Fatalf("session cookie set = %v, want %v", sessionToken != "", tt.wantSession)
			}
			if tt.wantSession {
				if user, err := authService.ValidateSession(context.Background(), sessionToken); err != nil || user == nil {
					t.Fatalf("session did not validate: user = %#v, err = %v", user, err)
				}
			}
		})
	}
}
