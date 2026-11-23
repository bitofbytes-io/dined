package repository

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/testutil"
	"github.com/google/uuid"
)

func TestPostgresUpdateVisitReplacesRatingsAndTags(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}

	input := postgresVisitInput(t, store, "Tag Diner")
	input.Ratings = map[uuid.UUID]float64{people[0].ID: 8, people[1].ID: 6.5}
	input.TagIDs = []uuid.UUID{tags[0].ID}
	input.NewTag = "Patio"
	visitID, err := store.CreateVisit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Ratings) != 2 || len(created.Tags) != 2 {
		t.Fatalf("created ratings = %#v, tags = %#v", created.Ratings, created.Tags)
	}

	update := postgresVisitInput(t, store, "")
	update.RestaurantID = &created.Restaurant.ID
	update.Ratings = map[uuid.UUID]float64{people[1].ID: 9.5}
	update.TagIDs = []uuid.UUID{tags[1].ID}
	update.NewTag = "Late Night"
	if err := store.UpdateVisit(ctx, *visitID, update); err != nil {
		t.Fatal(err)
	}

	updated, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Ratings) != 1 || updated.Ratings[0].Person.ID != people[1].ID || updated.Ratings[0].Score != 9.5 {
		t.Fatalf("updated ratings = %#v", updated.Ratings)
	}
	var tagNames []string
	for _, tag := range updated.Tags {
		tagNames = append(tagNames, tag.Name)
	}
	if want := []string{"Late Night", tags[1].Name}; !slices.Equal(tagNames, want) && !slices.Equal(tagNames, []string{want[1], want[0]}) {
		t.Fatalf("updated tags = %v, want %v", tagNames, want)
	}
}

func TestPostgresUpdateVisitReconcilesPhotos(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	input := postgresVisitInput(t, store, "Photo Diner")
	input.Photos = []model.VisitPhotoInput{
		{DataURI: testutil.PhotoURI(3)},
		{DataURI: testutil.PhotoURI(4)},
		{DataURI: testutil.PhotoURI(5)},
	}
	visitID, err := store.CreateVisit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Photos) != 3 {
		t.Fatalf("created photos = %d, want 3", len(created.Photos))
	}
	one, two, three := created.Photos[0], created.Photos[1], created.Photos[2]

	update := postgresVisitInput(t, store, "")
	update.RestaurantID = &created.Restaurant.ID
	update.KeepPhotoIDs = []uuid.UUID{three.ID, one.ID}
	update.Photos = []model.VisitPhotoInput{{DataURI: testutil.PhotoURI(6)}}
	if err := store.UpdateVisit(ctx, *visitID, update); err != nil {
		t.Fatal(err)
	}

	updated, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Photos) != 3 {
		t.Fatalf("updated photos = %#v", updated.Photos)
	}
	for i, want := range []model.VisitPhoto{three, one} {
		got := updated.Photos[i]
		if got.ID != want.ID || got.DataURI != want.DataURI || got.SortOrder != i || !got.CreatedAt.Equal(want.CreatedAt) {
			t.Fatalf("kept photo %d = %#v, want %#v at sort order %d", i, got, want, i)
		}
	}
	added := updated.Photos[2]
	if added.ID == one.ID || added.ID == two.ID || added.ID == three.ID || added.DataURI != "" || added.SortOrder != 2 || added.ByteCount != len(testutil.JPEG(8, 6, 6)) {
		t.Fatalf("new photo = %#v", added)
	}
	if removed, err := store.VisitPhoto(ctx, two.ID, ""); err != nil || removed != nil {
		t.Fatalf("removed photo = %#v, err = %v", removed, err)
	}

	update.KeepPhotoIDs = []uuid.UUID{two.ID}
	update.Photos = nil
	if err := store.UpdateVisit(ctx, *visitID, update); err == nil || !strings.Contains(err.Error(), "photo not found") {
		t.Fatalf("keeping a photo from outside the visit error = %v", err)
	}
	unchanged, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(unchanged.Photos) != 3 || unchanged.Photos[0].ID != three.ID {
		t.Fatalf("failed update should roll back photos: %#v", unchanged.Photos)
	}
}

func TestPostgresDeleteVisitCascadesAndFreesRestaurant(t *testing.T) {
	store := postgresStore(t)
	visitID, restaurantID := createPostgresVisit(t, store)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	deleted, err := store.DeleteRestaurantIfUnvisited(ctx, restaurantID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("restaurant with a visit should not be deleted")
	}

	if err := store.DeleteVisit(ctx, visitID); err != nil {
		t.Fatal(err)
	}
	if visit, err := store.Visit(ctx, visitID); err != nil || visit != nil {
		t.Fatalf("deleted visit = %#v, err = %v", visit, err)
	}
	for _, table := range []string{"visit_participant_ratings", "visit_tags", "visit_photos"} {
		if count := countPostgresRows(t, store, "SELECT COUNT(*) FROM "+table+" WHERE visit_id = $1", visitID); count != 0 {
			t.Fatalf("%s rows after delete = %d, want 0", table, count)
		}
	}

	deleted, err = store.DeleteRestaurantIfUnvisited(ctx, restaurantID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("restaurant without visits should be deleted")
	}
	if restaurant, err := store.Restaurant(ctx, restaurantID); err != nil || restaurant != nil {
		t.Fatalf("deleted restaurant = %#v, err = %v", restaurant, err)
	}
}
