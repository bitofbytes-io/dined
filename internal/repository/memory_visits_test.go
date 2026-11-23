package repository

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/testutil"
	"github.com/google/uuid"
)

func TestMemoryStoreVisitsZeroReturnsAllVisits(t *testing.T) {
	store := NewMemoryStore()

	limited, err := store.Visits(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 {
		t.Fatalf("expected 2 limited visits, got %d", len(limited))
	}

	all, err := store.Visits(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(store.visits) {
		t.Fatalf("expected all visits, got %d of %d", len(all), len(store.visits))
	}
}

func TestMemoryStoreVisitsNewestFirstUsesCreatedAtTieBreaker(t *testing.T) {
	store := NewMemoryStore()
	restaurant := store.restaurants[0]
	picker := &store.people[0]
	visitedAt := time.Date(2026, 5, 15, 18, 0, 0, 0, time.UTC)
	olderCreated := demoVisit(restaurant, picker, visitedAt, 2, "older", nil, nil)
	olderCreated.CreatedAt = visitedAt.Add(time.Minute)
	newerCreated := demoVisit(restaurant, picker, visitedAt, 2, "newer", nil, nil)
	newerCreated.CreatedAt = visitedAt.Add(2 * time.Minute)
	store.visits = []model.Visit{olderCreated, newerCreated}

	visits, err := store.Visits(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if visits[0].ID != newerCreated.ID {
		t.Fatalf("expected newest created visit first, got %s", visits[0].ID)
	}
}

func TestMemoryStoreVisitsPageOffsetsAndOmitsPhotoData(t *testing.T) {
	store := NewMemoryStore()
	store.visits[0].Photos = photosFromTestInput(t, store.visits[0].ID, []model.VisitPhotoInput{{DataURI: testVisitPhotoDataURI}}, 0)
	all, err := store.Visits(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}

	page, err := store.VisitsPage(context.Background(), model.VisitFilter{}, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].ID != all[1].ID || page[1].ID != all[2].ID {
		t.Fatalf("offset page = %#v", page)
	}
	if past, err := store.VisitsPage(context.Background(), model.VisitFilter{}, 2, len(all)); err != nil || len(past) != 0 {
		t.Fatalf("page past the end = %#v, err = %v", past, err)
	}

	listed := all[0]
	if listed.ID != store.visits[0].ID || len(listed.Photos) != 1 || listed.Photos[0].DataURI != "" || listed.Photos[0].ByteCount != len(testutil.JPEG(8, 6, 1)) {
		t.Fatalf("listed photos should carry metadata only: %#v", listed.Photos)
	}
	if store.visits[0].Photos[0].DataURI != testVisitPhotoDataURI {
		t.Fatal("listing visits must not clear stored photo data")
	}
	photo, err := store.VisitPhoto(context.Background(), listed.Photos[0].ID, "")
	if err != nil || photo == nil || !bytes.Equal(photo.Bytes, testutil.JPEG(8, 6, 1)) {
		t.Fatalf("visit photo = %#v, err = %v", photo, err)
	}
	if missing, err := store.VisitPhoto(context.Background(), uuid.New(), ""); err != nil || missing != nil {
		t.Fatalf("missing photo = %#v, err = %v", missing, err)
	}
}

func TestMemoryStoreVisitPositionMatchesVisitsOrder(t *testing.T) {
	store := NewMemoryStore()
	visits, err := store.Visits(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for want, visit := range visits {
		got, found, err := store.VisitPosition(context.Background(), visit.ID)
		if err != nil || !found || got != want {
			t.Fatalf("VisitPosition(%s) = %d, %v, %v; want %d", visit.ID, got, found, err, want)
		}
	}
	if _, found, err := store.VisitPosition(context.Background(), uuid.New()); err != nil || found {
		t.Fatalf("missing visit found = %v, err = %v", found, err)
	}
}

func TestMemoryStoreRestaurantVisitSummariesGroupsByRestaurant(t *testing.T) {
	store := NewMemoryStore()
	first, second := store.restaurants[0].ID, store.restaurants[1].ID

	summaries, err := store.RestaurantVisitSummaries(context.Background(), []uuid.UUID{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 || len(summaries[first]) != 1 || len(summaries[second]) != 1 {
		t.Fatalf("summaries = %#v", summaries)
	}
	if summaries[first][0].Restaurant.ID != first || summaries[second][0].Restaurant.ID != second {
		t.Fatalf("summaries grouped under wrong restaurant: %#v", summaries)
	}
}

func TestMemoryStoreRestaurantVisitSummariesOmitPhotos(t *testing.T) {
	store := NewMemoryStore()
	visit := store.visits[0]
	visit.Photos = photosFromTestInput(t, visit.ID, []model.VisitPhotoInput{{DataURI: testVisitPhotoDataURI}}, 0)
	store.visits[0] = visit

	summariesByRestaurant, err := store.RestaurantVisitSummaries(context.Background(), []uuid.UUID{visit.Restaurant.ID})
	if err != nil {
		t.Fatal(err)
	}
	summaries := summariesByRestaurant[visit.Restaurant.ID]
	if len(summaries) == 0 {
		t.Fatal("expected restaurant visit summaries")
	}
	if len(summaries[0].Photos) != 0 {
		t.Fatalf("summary photos len = %d, want 0", len(summaries[0].Photos))
	}

	fullVisits, err := store.RestaurantVisits(context.Background(), visit.Restaurant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fullVisits) == 0 || len(fullVisits[0].Photos) != 1 {
		t.Fatalf("full restaurant visits should still include photos: %#v", fullVisits)
	}
}
