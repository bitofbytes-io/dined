package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func (m *MemoryStore) CreateVisit(ctx context.Context, input model.VisitInput) (*uuid.UUID, error) {
	input, photos, err := prepareVisitInput(ctx, input)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Check the picker before creating a restaurant, as Postgres rolls both back.
	picker, err := m.pickerFromInput(input.PickerID)
	if err != nil {
		return nil, err
	}
	restaurant, err := m.resolveRestaurant(input)
	if err != nil {
		return nil, err
	}

	visit := model.Visit{
		ID:         uuid.New(),
		Restaurant: restaurant,
		VisitedAt:  input.VisitedAt,
		Picker:     picker,
		PriceLevel: input.PriceLevel,
		Notes:      strPtrOrNil(input.Notes),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	visit.Ratings = m.ratingsFromInput(input)
	visit.Photos = photosFromPrepared(visit.ID, photos, 0)
	for _, tagID := range input.TagIDs {
		if tag, ok := m.tagByID(tagID); ok {
			visit.Tags = append(visit.Tags, tag)
		}
	}
	if name := strings.TrimSpace(input.NewTag); name != "" {
		tag := model.Tag{ID: uuid.New(), Name: name}
		m.tags = append(m.tags, tag)
		visit.Tags = append(visit.Tags, tag)
	}
	m.visits = append(m.visits, visit)
	m.markWantToTryDone(restaurant.ID)
	return &visit.ID, nil
}

func (m *MemoryStore) UpdateVisit(ctx context.Context, id uuid.UUID, input model.VisitInput) error {
	input, preparedPhotos, err := prepareVisitInput(ctx, input)
	if err != nil {
		return err
	}
	if input.RestaurantID == nil {
		return errors.New("restaurant is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	restaurant, ok := m.findRestaurant(input.RestaurantID)
	if !ok {
		return errors.New("restaurant not found")
	}
	picker, err := m.pickerFromInput(input.PickerID)
	if err != nil {
		return err
	}
	visitIndex := -1
	for i := range m.visits {
		if m.visits[i].ID == id {
			visitIndex = i
			break
		}
	}
	if visitIndex == -1 {
		return errors.New("visit not found")
	}

	ratings := m.ratingsFromInput(input)

	photos, err := m.reconciledVisitPhotos(m.visits[visitIndex], input, preparedPhotos)
	if err != nil {
		return err
	}

	tagIDs := append([]uuid.UUID(nil), input.TagIDs...)
	if name := strings.TrimSpace(input.NewTag); name != "" {
		tag := model.Tag{ID: uuid.New(), Name: name}
		m.tags = append(m.tags, tag)
		tagIDs = append(tagIDs, tag.ID)
	}
	var tags []model.Tag
	for _, tagID := range tagIDs {
		if tag, ok := m.tagByID(tagID); ok {
			tags = append(tags, tag)
		}
	}

	// Like Postgres, only a dine moved to another restaurant clears that one's item.
	if m.visits[visitIndex].Restaurant.ID != restaurant.ID {
		m.markWantToTryDone(restaurant.ID)
	}
	m.visits[visitIndex].Restaurant = restaurant
	m.visits[visitIndex].VisitedAt = input.VisitedAt
	m.visits[visitIndex].Picker = picker
	m.visits[visitIndex].PriceLevel = input.PriceLevel
	m.visits[visitIndex].Notes = strPtrOrNil(input.Notes)
	m.visits[visitIndex].Ratings = ratings
	m.visits[visitIndex].Tags = tags
	m.visits[visitIndex].Photos = photos
	m.visits[visitIndex].UpdatedAt = time.Now()
	return nil
}

func (m *MemoryStore) personByID(id uuid.UUID) model.Person {
	for _, person := range m.people {
		if person.ID == id {
			return person
		}
	}
	return model.Person{}
}

// pickerFromInput returns the picker a visit input names: nil for Everybody,
// or model.ErrUnknownPicker when the ID is not one of the people.
func (m *MemoryStore) pickerFromInput(id *uuid.UUID) (*model.Person, error) {
	if id == nil {
		return nil, nil
	}
	for _, person := range m.people {
		if person.ID == *id {
			picker := person
			return &picker, nil
		}
	}
	return nil, model.ErrUnknownPicker
}

func (m *MemoryStore) ratingsFromInput(input model.VisitInput) []model.Rating {
	var ratings []model.Rating
	seen := map[uuid.UUID]struct{}{}
	for _, person := range m.people {
		score, ok := input.Ratings[person.ID]
		if !ok {
			continue
		}
		ratings = append(ratings, model.Rating{Person: person, Score: score})
		seen[person.ID] = struct{}{}
	}
	for personID, score := range input.Ratings {
		if _, ok := seen[personID]; ok {
			continue
		}
		ratings = append(ratings, model.Rating{Person: m.personByID(personID), Score: score})
	}
	return ratings
}

func (m *MemoryStore) tagByID(id uuid.UUID) (model.Tag, bool) {
	for _, tag := range m.tags {
		if tag.ID == id {
			return tag, true
		}
	}
	return model.Tag{}, false
}
