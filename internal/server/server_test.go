package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
)

func TestRouterDeletesUnvisitedRestaurantAndPreservesSearch(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Amigos",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	restaurants, err := store.Restaurants(ctx, "Amigos")
	if err != nil {
		t.Fatal(err)
	}
	if len(restaurants) != 1 {
		t.Fatalf("expected one Amigos restaurant, got %d", len(restaurants))
	}
	if err := store.DeleteVisit(ctx, *visitID); err != nil {
		t.Fatal(err)
	}

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+restaurants[0].ID.String()+"/delete", strings.NewReader("q=Amigos"))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/search?q=Amigos" {
		t.Fatalf("redirect location = %q, want /search?q=Amigos", location)
	}
	remaining, err := store.Restaurants(ctx, "Amigos")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected Amigos to be deleted, got %#v", remaining)
	}
}

func TestRouterRefusesVisitedRestaurantDelete(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	restaurants, err := store.Restaurants(ctx, "Hank")
	if err != nil {
		t.Fatal(err)
	}
	if len(restaurants) != 1 {
		t.Fatalf("expected one Hank restaurant, got %d", len(restaurants))
	}

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+restaurants[0].ID.String()+"/delete", strings.NewReader("q=Hank"))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusConflict)
	}
	remaining, err := store.Restaurants(ctx, "Hank")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected visited restaurant to remain, got %#v", remaining)
	}
}

func TestRouterUpdateRestaurantReturnsToEditDine(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Return Flow Diner",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	visit, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if visit == nil {
		t.Fatal("expected created visit")
	}

	form := url.Values{}
	form.Set("restaurant_name", "Return Flow Diner Updated")
	form.Set("return_visit_id", visitID.String())

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+visit.Restaurant.ID.String(), strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	wantLocation := "/visits/" + visitID.String() + "/edit"
	if location := rec.Header().Get("Location"); location != wantLocation {
		t.Fatalf("redirect location = %q, want %q", location, wantLocation)
	}
	updated, err := store.Restaurant(ctx, visit.Restaurant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil || updated.Name != "Return Flow Diner Updated" {
		t.Fatalf("restaurant was not updated: %#v", updated)
	}
}

func TestRouterUpdateRestaurantIgnoresMismatchedReturnVisitID(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstVisitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "First Diner",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondVisitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Second Diner",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstVisit, err := store.Visit(ctx, *firstVisitID)
	if err != nil {
		t.Fatal(err)
	}
	if firstVisit == nil {
		t.Fatal("expected first visit")
	}

	form := url.Values{}
	form.Set("restaurant_name", "First Diner Updated")
	form.Set("return_visit_id", secondVisitID.String())

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+firstVisit.Restaurant.ID.String(), strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	wantLocation := "/restaurants/" + firstVisit.Restaurant.ID.String()
	if location := rec.Header().Get("Location"); location != wantLocation {
		t.Fatalf("redirect location = %q, want %q", location, wantLocation)
	}
}

func TestRouterGoogleRefreshRedirectsToEditWithReturnVisitIDWhenUnconfigured(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Google Return Diner",
		GooglePlaceID:  "place-1",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	visit, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if visit == nil {
		t.Fatal("expected created visit")
	}

	form := url.Values{}
	form.Set("return_visit_id", visitID.String())

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+visit.Restaurant.ID.String()+"/google-refresh", strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	wantLocation := "/restaurants/" + visit.Restaurant.ID.String() + "/edit?google_refresh=unconfigured&return_visit_id=" + visitID.String()
	if location := rec.Header().Get("Location"); location != wantLocation {
		t.Fatalf("redirect location = %q, want %q", location, wantLocation)
	}
}

func TestRouterCreateVisitWithoutRatingPreservesPostedForm(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("restaurant_name", "Tupelo Honey")
	form.Set("address", "123 Main St")
	form.Set("city", "Apex")
	form.Set("google_place_id", "place-1")
	form.Set("category", "American")
	form.Set("visited_at", "2026-05-17T20:50")
	form.Set("picker_id", people[1].ID.String())
	form.Set("price_level", "3")
	form.Set("notes", "Dinner notes")
	form.Set("new_tag", "Patio")
	form.Set("is_chain", "true")
	form.Add("tag_id", tags[0].ID.String())

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/visits", strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	rendered := rec.Body.String()
	for _, fragment := range []string{
		"at least one rating is required",
		`name="restaurant_name" list="restaurant-options" placeholder="Search or add restaurant" value="Tupelo Honey" required`,
		`name="address" placeholder="Optional" value="123 Main St"`,
		`name="city" value="Apex"`,
		`name="google_place_id" placeholder="Optional" value="place-1"`,
		`<option selected>American</option>`,
		`name="visited_at" value="2026-05-17T20:50" required`,
		`value="` + people[1].ID.String() + `" selected>` + people[1].Name + `</option>`,
		`value="3" selected>$$$</option>`,
		`name="tag_id" value="` + tags[0].ID.String() + `" checked`,
		`name="new_tag" placeholder="Great fries" value="Patio"`,
		`Dinner notes</textarea>`,
		`name="is_chain" value="true" checked`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("response missing %q:\n%s", fragment, rendered)
		}
	}

	after, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("visit count = %d, want %d", len(after), len(before))
	}
}

func TestRouterCreateVisitAcceptsZeroRating(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("restaurant_name", "Zero Star Diner")
	form.Set("visited_at", "2026-05-17T20:50")
	form.Set("picker_id", people[0].ID.String())
	form.Set("price_level", "2")
	form.Set("rating_"+people[0].ID.String(), "0")

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/visits", strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	after, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("visit count = %d, want %d", len(after), len(before)+1)
	}
	created := visitByRestaurantName(after, "Zero Star Diner")
	if created == nil {
		t.Fatalf("created visit not found in %#v", after)
	}
	if len(created.Ratings) != 1 || created.Ratings[0].Score != 0 {
		t.Fatalf("created ratings = %#v, want one zero rating", created.Ratings)
	}
}

func TestRouterCreateVisitStoresPhotos(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("restaurant_name", "Snapshot Diner")
	form.Set("visited_at", "2026-05-17T20:50")
	form.Set("picker_id", people[0].ID.String())
	form.Set("price_level", "2")
	form.Set("rating_"+people[0].ID.String(), "8")
	form.Add("photo_data_uri", "data:image/jpeg;base64,aGVsbG8=")
	form.Add("photo_data_uri", "data:image/jpeg;base64,dGFjbw==")

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/visits", strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	visits, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	created := visitByRestaurantName(visits, "Snapshot Diner")
	if created == nil {
		t.Fatalf("created visit not found in %#v", visits)
	}
	if len(created.Photos) != 2 {
		t.Fatalf("photo len = %d, want 2: %#v", len(created.Photos), created.Photos)
	}
	if created.Photos[0].ContentType != "image/jpeg" || created.Photos[0].ByteCount != 5 {
		t.Fatalf("first photo = %#v", created.Photos[0])
	}
}

func visitByRestaurantName(visits []model.Visit, name string) *model.Visit {
	for i := range visits {
		if visits[i].Restaurant.Name == name {
			return &visits[i]
		}
	}
	return nil
}

func TestRouterUpdateVisitErrorPreservesPostedForm(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Edit Error Diner",
		VisitedAt:      time.Date(2026, 5, 10, 18, 0, 0, 0, time.UTC),
		PickerID:       people[0].ID,
		PriceLevel:     1,
		Notes:          "Saved notes",
		Ratings:        map[uuid.UUID]float64{people[0].ID: 6},
		TagIDs:         []uuid.UUID{tags[0].ID},
		Photos: []model.VisitPhotoInput{
			{DataURI: "data:image/jpeg;base64,a2VlcA=="},
			{DataURI: "data:image/jpeg;base64,ZHJvcA=="},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Visit(ctx, *visitID)
	if err != nil || saved == nil {
		t.Fatalf("saved visit = %#v, err = %v", saved, err)
	}
	keptPhoto, removedPhoto := saved.Photos[0], saved.Photos[1]
	const newPhoto = "data:image/jpeg;base64,bmV3"

	form := url.Values{}
	form.Set("restaurant_id", saved.Restaurant.ID.String())
	form.Set("visited_at", "2026-05-17T20:50")
	form.Set("picker_id", people[1].ID.String())
	form.Set("price_level", "4")
	form.Set("notes", "Edited notes")
	form.Set("new_tag", "Patio")
	form.Set("rating_"+people[1].ID.String(), "") // Every rating cleared: validation fails.
	form.Add("tag_id", tags[1].ID.String())
	form.Add("keep_photo_id", keptPhoto.ID.String())
	form.Add("photo_data_uri", newPhoto)

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/visits/"+visitID.String(), strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	rendered := rec.Body.String()
	for _, fragment := range []string{
		"at least one rating is required",
		`name="visited_at" value="2026-05-17T20:50"`,
		`value="` + people[1].ID.String() + `" selected>` + people[1].Name + `</option>`,
		`value="4" selected>$$$$</option>`,
		`name="tag_id" value="` + tags[1].ID.String() + `" checked`,
		`name="new_tag" placeholder="Great fries" value="Patio"`,
		`Edited notes</textarea>`,
		`name="keep_photo_id" value="` + keptPhoto.ID.String() + `"`,
		`name="photo_data_uri" value="` + newPhoto + `"`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("response missing %q:\n%s", fragment, rendered)
		}
	}
	for _, fragment := range []string{
		`value="` + people[0].ID.String() + `" selected>`,
		`name="tag_id" value="` + tags[0].ID.String() + `" checked`,
		`name="rating_` + people[0].ID.String() + `" type="number" min="0" max="10" step="0.5" inputmode="decimal" placeholder="0-10" value="6"`,
		`name="keep_photo_id" value="` + removedPhoto.ID.String() + `"`,
		"Saved notes",
	} {
		if strings.Contains(rendered, fragment) {
			t.Fatalf("response should not restore saved value %q:\n%s", fragment, rendered)
		}
	}

	unchanged, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Picker.ID != people[0].ID || len(unchanged.Photos) != 2 || unchanged.Notes == nil || *unchanged.Notes != "Saved notes" {
		t.Fatalf("failed update changed the saved visit: %#v", unchanged)
	}
}

type countingAuthRepository struct {
	*auth.MemoryRepository
	sessionLookups int
}

func (c *countingAuthRepository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (*auth.Session, *auth.User, error) {
	c.sessionLookups++
	return c.MemoryRepository.FindSessionByTokenHash(ctx, tokenHash)
}

func TestRouterLooksUpSessionOncePerPage(t *testing.T) {
	repo := &countingAuthRepository{MemoryRepository: auth.NewMemoryRepository()}
	authService := auth.NewService(repo, time.Hour, nil)
	user, err := authService.CreateOrUpdateUser(context.Background(), &auth.GoogleClaims{Sub: "google-user", Email: "family@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	token, err := authService.CreateSession(context.Background(), user.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	router := New(&config.Config{AuthSessionTTL: time.Hour}, repository.NewMemoryStore(), places.NewClient(""), authService, nil).Router()

	for _, path := range []string{"/log", "/dines"} {
		repo.sessionLookups = 0
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "LOGOUT") {
			t.Fatalf("%s did not render the signed-in navigation", path)
		}
		if repo.sessionLookups != 1 {
			t.Fatalf("%s looked up the session %d times, want 1", path, repo.sessionLookups)
		}
	}
}

func TestRouterCreateVisitRejectsInvalidGoogleRating(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	router, token := newAuthenticatedTestRouter(t, store)

	for value, want := range map[string]string{
		"999": "Google rating must be between 0 and 5",
		"NaN": "Google rating must be a finite number",
	} {
		form := url.Values{}
		form.Set("restaurant_name", "Rating Diner")
		form.Set("visited_at", "2026-05-17T20:50")
		form.Set("picker_id", people[0].ID.String())
		form.Set("price_level", "2")
		form.Set("rating_"+people[0].ID.String(), "8")
		form.Set("google_rating", value)

		rec := postVisitForm(t, router, token, "/visits", form)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("google_rating=%s: status %d, want error %q in:\n%s", value, rec.Code, want, rec.Body.String())
		}
	}
	if restaurants, err := store.Restaurants(ctx, "Rating Diner"); err != nil || len(restaurants) != 0 {
		t.Fatalf("invalid Google rating saved a restaurant: %#v, err = %v", restaurants, err)
	}
}

func TestRouterUpdateVisitShowsGenericErrorWhenStoreFails(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Store Error Diner",
		VisitedAt:      time.Date(2026, 5, 10, 18, 0, 0, 0, time.UTC),
		PickerID:       people[0].ID,
		PriceLevel:     1,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("restaurant_id", saved.Restaurant.ID.String())
	form.Set("visited_at", "2026-05-17T20:50")
	form.Set("picker_id", people[0].ID.String())
	form.Set("price_level", "2")
	form.Set("rating_"+people[0].ID.String(), "7")
	form.Add("keep_photo_id", uuid.NewString()) // Not one of this visit's photos: the store rejects it.

	router, token := newAuthenticatedTestRouter(t, store)
	rec := postVisitForm(t, router, token, "/visits/"+visitID.String(), form)

	rendered := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(rendered, "Could not save this dine. Please try again.") {
		t.Fatalf("status %d, want generic save error in:\n%s", rec.Code, rendered)
	}
	if strings.Contains(rendered, "photo not found") {
		t.Fatalf("response leaked the store error:\n%s", rendered)
	}
}
