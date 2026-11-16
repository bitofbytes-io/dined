package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderHomeHasNoPickerTurn(t *testing.T) {
	var out strings.Builder
	if err := Render(&out, "home", PageData{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Next Up") {
		t.Fatalf("rendered home still shows a picker turn:\n%s", out.String())
	}
}

func TestRenderHomeHeroUsesArrowSignForBothAudiences(t *testing.T) {
	for _, tc := range []struct {
		name          string
		authenticated bool
		want          string
		other         string
	}{
		{"logged out", false, `<a class="image-cta" href="/api/auth/google" hx-boost="false"><img src="/static/assets/dined-log-in-arrow-generated.webp" alt="Log In"></a>`, `href="/log"`},
		{"logged in", true, `<a class="image-cta" href="/log"><img src="/static/assets/dined-log-arrow-generated.png" alt="Log a Dine"></a>`, `alt="Log In"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			if err := Render(&out, "home", PageData{Authenticated: tc.authenticated}); err != nil {
				t.Fatal(err)
			}
			rendered := out.String()
			if !strings.Contains(rendered, tc.want) {
				t.Fatalf("rendered home missing %q:\n%s", tc.want, rendered)
			}
			if strings.Contains(rendered, tc.other) || strings.Contains(rendered, "public-badge-link") {
				t.Fatalf("rendered home shows the other audience's hero button:\n%s", rendered)
			}
		})
	}
	assetInfo, err := os.Stat(filepath.Join("..", "..", "static", "assets", "dined-log-in-arrow-generated.webp"))
	if err != nil {
		t.Fatalf("Log In sign asset missing: %v", err)
	}
	if assetInfo.Size() >= 60_000 {
		t.Fatalf("Log In sign is %d bytes, want less than 60 KB", assetInfo.Size())
	}
}
