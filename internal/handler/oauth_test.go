package handler

import "testing"

func TestSafeRedirectPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/log", want: true},
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
