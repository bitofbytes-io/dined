package repository

import (
	"bytes"
	"context"
	"sort"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

// memoryWantToTry is a stored Want to Try item. Its restaurant and suggester
// are looked up on read so edits to them show, as Postgres joins them.
type memoryWantToTry struct {
	id            uuid.UUID
	restaurantID  uuid.UUID
	suggestedByID *uuid.UUID
	note          *string
	createdAt     time.Time
	doneAt        *time.Time
}

func (m *MemoryStore) WantToTry(context.Context) ([]model.WantToTryItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var items []model.WantToTryItem
	for _, stored := range m.wantToTry {
		if stored.doneAt != nil {
			continue
		}
		if item, ok := m.wantToTryItem(stored); ok {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return bytes.Compare(items[i].ID[:], items[j].ID[:]) < 0
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func (m *MemoryStore) WantToTryItem(_ context.Context, id uuid.UUID) (*model.WantToTryItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, stored := range m.wantToTry {
		if stored.id != id {
			continue
		}
		if item, ok := m.wantToTryItem(stored); ok {
			return &item, nil
		}
	}
	return nil, nil
}

// SaveWantToTry mirrors the Postgres store: it finds or creates the restaurant
// like a new dine, replaces the suggester and note of a listed restaurant, and
// puts a restaurant a dine took off the list back on as a new item.
func (m *MemoryStore) SaveWantToTry(_ context.Context, input model.WantToTryInput) (*uuid.UUID, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check before creating a restaurant, as Postgres rolls both back.
	if _, err := m.pickerFromInput(input.SuggestedByID); err != nil {
		return nil, err
	}
	if _, ok := m.findRestaurant(input.RestaurantID); input.RestaurantID != nil && !ok {
		return nil, model.ErrRestaurantNotFound
	}
	restaurant, err := m.resolveRestaurant(input.VisitInput())
	if err != nil {
		return nil, err
	}

	var suggestedByID *uuid.UUID
	if input.SuggestedByID != nil {
		id := *input.SuggestedByID
		suggestedByID = &id
	}
	now := time.Now()
	for i := range m.wantToTry {
		stored := &m.wantToTry[i]
		if stored.restaurantID != restaurant.ID {
			continue
		}
		stored.suggestedByID = suggestedByID
		stored.note = strPtrOrNil(input.Note)
		if stored.doneAt != nil {
			stored.createdAt = now
			stored.doneAt = nil
		}
		return &stored.id, nil
	}
	stored := memoryWantToTry{
		id:            uuid.New(),
		restaurantID:  restaurant.ID,
		suggestedByID: suggestedByID,
		note:          strPtrOrNil(input.Note),
		createdAt:     now,
	}
	m.wantToTry = append(m.wantToTry, stored)
	return &stored.id, nil
}

func (m *MemoryStore) DeleteWantToTry(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, stored := range m.wantToTry {
		if stored.id == id {
			m.wantToTry = append(m.wantToTry[:i], m.wantToTry[i+1:]...)
			return nil
		}
	}
	return nil
}

// markWantToTryDone takes a restaurant off the list because a dine was
// logged there. Callers hold m.mu for writing.
func (m *MemoryStore) markWantToTryDone(restaurantID uuid.UUID) {
	now := time.Now()
	for i := range m.wantToTry {
		if m.wantToTry[i].restaurantID == restaurantID && m.wantToTry[i].doneAt == nil {
			m.wantToTry[i].doneAt = &now
		}
	}
}

// deleteRestaurantWantToTry drops a deleted restaurant's item, as the Postgres
// foreign key cascades. Callers hold m.mu for writing.
func (m *MemoryStore) deleteRestaurantWantToTry(restaurantID uuid.UUID) {
	kept := m.wantToTry[:0]
	for _, stored := range m.wantToTry {
		if stored.restaurantID != restaurantID {
			kept = append(kept, stored)
		}
	}
	m.wantToTry = kept
}

// wantToTryItem joins a stored item with its restaurant and suggester.
// Callers hold m.mu.
func (m *MemoryStore) wantToTryItem(stored memoryWantToTry) (model.WantToTryItem, bool) {
	restaurant, ok := m.findRestaurant(&stored.restaurantID)
	if !ok {
		return model.WantToTryItem{}, false
	}
	item := model.WantToTryItem{
		ID:         stored.id,
		Restaurant: restaurant,
		CreatedAt:  stored.createdAt,
	}
	if stored.note != nil {
		note := *stored.note
		item.Note = &note
	}
	if stored.doneAt != nil {
		doneAt := *stored.doneAt
		item.DoneAt = &doneAt
	}
	if stored.suggestedByID != nil {
		suggester := m.personByID(*stored.suggestedByID)
		item.SuggestedBy = &suggester
	}
	return item, true
}
