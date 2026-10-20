package repository

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/testutil"
	"github.com/google/uuid"
)

// Done is evaluated when photo preparation selects on the decoder semaphore.
// The observation returns immediately and does not change cancellation behavior.
type preparationContext struct {
	context.Context
	once    sync.Once
	observe func()
}

func (c *preparationContext) Done() <-chan struct{} {
	c.once.Do(c.observe)
	return c.Context.Done()
}

func TestMemoryPhotoPreparationAllowsUnrelatedReads(t *testing.T) {
	for _, update := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "update"}[update], func(t *testing.T) {
			store := NewMemoryStore()
			input := model.VisitInput{RestaurantName: "Prepared diner", VisitedAt: time.Now(), PriceLevel: 2, Ratings: map[uuid.UUID]float64{store.people[0].ID: 8}}
			var id *uuid.UUID
			if update {
				var err error
				id, err = store.CreateVisit(context.Background(), input)
				if err != nil {
					t.Fatal(err)
				}
				visit, err := store.Visit(context.Background(), *id)
				if err != nil {
					t.Fatal(err)
				}
				input.RestaurantID = &visit.Restaurant.ID
			}
			input.Photos = []model.VisitPhotoInput{{DataURI: testutil.PhotoURI(1)}}
			observed := false
			ctx := &preparationContext{Context: context.Background(), observe: func() {
				observed = true
				if !store.mu.TryRLock() {
					t.Error("photo preparation holds the memory-store write lock")
					return
				}
				store.mu.RUnlock()
				if people, err := store.People(context.Background()); err != nil || len(people) == 0 {
					t.Error("unrelated memory read failed while preparing", err)
				}
			}}
			if update {
				if err := store.UpdateVisit(ctx, *id, input); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := store.CreateVisit(ctx, input); err != nil {
					t.Fatal(err)
				}
			}
			if !observed {
				t.Fatal("photo preparation did not use the caller's context")
			}
		})
	}
}

func TestMemoryPhotoPreparationFailureDoesNotWrite(t *testing.T) {
	for _, update := range []bool{false, true} {
		for _, reason := range []string{"cancelled preparation", "invalid second image", "unknown kept photo"} {
			if !update && reason == "unknown kept photo" {
				continue
			}
			t.Run(map[bool]string{false: "create/", true: "update/"}[update]+reason, func(t *testing.T) {
				store := NewMemoryStore()
				input := model.VisitInput{RestaurantName: "Atomic photo diner", VisitedAt: time.Now(), PriceLevel: 2, Ratings: map[uuid.UUID]float64{store.people[0].ID: 8}, Photos: []model.VisitPhotoInput{{DataURI: testutil.PhotoURI(1)}}}
				id, err := store.CreateVisit(context.Background(), input)
				if err != nil {
					t.Fatal(err)
				}
				visit, err := store.Visit(context.Background(), *id)
				if err != nil {
					t.Fatal(err)
				}
				if update {
					input.RestaurantID = &visit.Restaurant.ID
					input.KeepPhotoIDs = []uuid.UUID{visit.Photos[0].ID}
				} else {
					input.RestaurantName = "Should never be created"
				}
				input.NewTag = "Should never be added"
				snapshot := func() []byte {
					b, err := json.Marshal([]any{store.tags, store.restaurants, store.visits, store.wantToTry})
					if err != nil {
						t.Fatal(err)
					}
					return b
				}
				before := snapshot()
				ctx := context.Background()
				switch reason {
				case "cancelled preparation":
					cancelled, cancel := context.WithCancel(ctx)
					defer cancel()
					ctx = &preparationContext{Context: cancelled, observe: cancel}
				case "invalid second image":
					input.Photos = append(input.Photos, model.VisitPhotoInput{DataURI: "data:image/jpeg;base64,aGVsbG8="})
				case "unknown kept photo":
					input.KeepPhotoIDs = []uuid.UUID{uuid.New()}
				}
				if update {
					err = store.UpdateVisit(ctx, *id, input)
				} else {
					_, err = store.CreateVisit(ctx, input)
				}
				if err == nil {
					t.Fatal("expected failure")
				}
				if reason == "cancelled preparation" && !errors.Is(err, context.Canceled) {
					t.Fatal("wrong cancellation error", err)
				}
				if string(before) != string(snapshot()) {
					t.Fatal("failed preparation changed memory-store state")
				}
			})
		}
	}
}
