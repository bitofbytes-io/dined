package repository

import (
	"context"
	"github.com/bitofbytes-io/dined/internal/model"
)

// Validation, JPEG work, and ownership copies all finish before a store takes
// locks or starts a transaction. Handler-prepared inputs reuse the private cache.
func prepareVisitInput(ctx context.Context, input model.VisitInput) (model.VisitInput, []model.VisitPhoto, error) {
	input, err := input.Prepare(ctx)
	if err != nil {
		return model.VisitInput{}, nil, err
	}
	photos, err := input.PreparedPhotos()
	if err != nil {
		return model.VisitInput{}, nil, err
	}
	return input, photos, nil
}
