package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
)

func pickForm(people []model.Person, name, picker string) url.Values {
	form := url.Values{}
	form.Set("restaurant_name", name)
	form.Set("visited_at", "2026-09-27T18:00")
	if picker != "" {
		form.Set("picker_id", picker)
	}
	form.Set("price_level", "2")
	form.Set("rating_"+people[0].ID.String(), "8")
	form.Set("rating_"+people[1].ID.String(), "9")
	return form
}

func TestRouterLogPageDefaultsPickerToEverybody(t *testing.T) {
	router, token := newAuthenticatedTestRouter(t, repository.NewMemoryStore())
	req := httptest.NewRequest(http.MethodGet, "/log", nil)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `name="picker_id" value="everybody" checked>`) {
		t.Fatalf("status %d, want Everybody checked:\n%s", rec.Code, body)
	}
	if strings.Count(body, `name="picker_id"`) != 5 || strings.Count(body, `name="picker_id" value="everybody" checked>`) != 1 {
		t.Fatalf("log should offer Everybody and four people with only Everybody checked:\n%s", body)
	}
}

func TestRouterCreateVisitAcceptsEverybodyAndBackToBackPicks(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	router, token := newAuthenticatedTestRouter(t, store)

	for _, test := range []struct{ name, picker string }{
		{"Everybody Diner", "everybody"},
		{"No Picker Diner", ""},
		{"Daniel Once", people[0].ID.String()},
		{"Daniel Twice", people[0].ID.String()},
	} {
		rec := postVisitForm(t, router, token, "/visits", pickForm(people, test.name, test.picker))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s: status %d, want %d:\n%s", test.name, rec.Code, http.StatusSeeOther, rec.Body.String())
		}
	}

	visits, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	pickers := map[string]string{}
	for _, visit := range visits {
		pickers[visit.Restaurant.Name] = "Everybody"
		if visit.Picker != nil {
			pickers[visit.Restaurant.Name] = visit.Picker.Name
		}
	}
	for name, want := range map[string]string{
		"Everybody Diner": "Everybody",
		"No Picker Diner": "Everybody",
		"Daniel Once":     "Daniel",
		"Daniel Twice":    "Daniel",
	} {
		if pickers[name] != want {
			t.Fatalf("%s picker = %q, want %q", name, pickers[name], want)
		}
	}

	page := getPublic(router, "/dines").Body.String()
	if got := strings.Count(page, "Picked by Everybody"); got != 2 {
		t.Fatalf("/dines shows %d Everybody picks, want 2:\n%s", got, page)
	}
	// The seeded demo dine is also Daniel's.
	if got := strings.Count(page, "Picked by Daniel"); got != 3 {
		t.Fatalf("/dines shows %d Daniel picks, want 3:\n%s", got, page)
	}
}

func TestRouterRejectsInvalidPickerWithBadRequest(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	router, token := newAuthenticatedTestRouter(t, store)
	before, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, picker := range []string{"nobody", "Everybody", uuid.NewString()} {
		rec := postVisitForm(t, router, token, "/visits", pickForm(people, "Bad Picker Diner", picker))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Picked by must be Everybody or one of the family.") {
			t.Fatalf("create picker=%q: status %d, want 400 with a friendly message: %s", picker, rec.Code, rec.Body.String())
		}

		edit := pickForm(people, "", picker)
		edit.Set("restaurant_id", before[0].Restaurant.ID.String())
		rec = postVisitForm(t, router, token, "/visits/"+before[0].ID.String(), edit)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Picked by must be Everybody or one of the family.") {
			t.Fatalf("update picker=%q: status %d, want 400 with a friendly message: %s", picker, rec.Code, rec.Body.String())
		}
	}

	after, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("invalid pickers created visits: %d, want %d", len(after), len(before))
	}
	if restaurants, err := store.Restaurants(ctx, "Bad Picker Diner"); err != nil || len(restaurants) != 0 {
		t.Fatalf("invalid pickers created restaurants: %#v, err = %v", restaurants, err)
	}
	unchanged, err := store.Visit(ctx, before[0].ID)
	if err != nil || unchanged.Picker == nil || unchanged.Picker.ID != before[0].Picker.ID {
		t.Fatalf("invalid picker changed the visit: %#v, err = %v", unchanged, err)
	}
}

func TestRouterEditVisitChangesPicker(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	router, token := newAuthenticatedTestRouter(t, store)
	visits, err := store.Visits(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	visit := visits[0]

	editPage := func() string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/visits/"+visit.ID.String()+"/edit", nil)
		req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	if page := editPage(); !strings.Contains(page, `name="picker_id" value="`+visit.Picker.ID.String()+`" checked>`) {
		t.Fatalf("edit page should check the stored picker:\n%s", page)
	}

	for _, picker := range []string{"everybody", people[3].ID.String()} {
		form := pickForm(people, "", picker)
		form.Set("restaurant_id", visit.Restaurant.ID.String())
		if rec := postVisitForm(t, router, token, "/visits/"+visit.ID.String(), form); rec.Code != http.StatusSeeOther {
			t.Fatalf("edit picker=%s: status %d:\n%s", picker, rec.Code, rec.Body.String())
		}
		if page := editPage(); !strings.Contains(page, `name="picker_id" value="`+picker+`" checked>`) {
			t.Fatalf("edit page should check picker %s after saving:\n%s", picker, page)
		}
	}
}
