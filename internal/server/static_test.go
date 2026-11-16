package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRevalidateModulesOnlyMarksFeatureModules(t *testing.T) {
	handler := revalidateModules(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/js/photos.js", want: "no-cache"},
		{path: "/app.js", want: ""},
		{path: "/styles.css", want: ""},
	} {
		// http.StripPrefix leaves the path without its leading "/static/".
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.URL.Path = tc.path[1:]
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if got := rec.Header().Get("Cache-Control"); got != tc.want {
			t.Errorf("%s Cache-Control = %q, want %q", tc.path, got, tc.want)
		}
	}
}
