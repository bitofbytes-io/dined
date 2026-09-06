package handler

import "testing"

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
