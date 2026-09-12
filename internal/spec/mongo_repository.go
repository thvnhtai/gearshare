package spec

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collectionName = "listing_specs"

type Repository struct {
	collection *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{collection: db.Collection(collectionName)}
}

// Upsert replaces the whole spec document for a listing — spec sheets are
// edited as a unit from the owner's listing-edit form, not field-by-field,
// so there's no partial-update use case here.
func (r *Repository) Upsert(ctx context.Context, s *Spec) error {
	s.UpdatedAt = time.Now()
	_, err := r.collection.ReplaceOne(ctx,
		bson.M{"listing_id": s.ListingID},
		s,
		options.Replace().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("spec: upsert: %w", err)
	}
	return nil
}

func (r *Repository) GetByListingID(ctx context.Context, listingID int64) (*Spec, error) {
	var s Spec
	err := r.collection.FindOne(ctx, bson.M{"listing_id": listingID}).Decode(&s)
	if err == mongo.ErrNoDocuments {
		return nil, nil // no spec sheet is a valid state, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("spec: get: %w", err)
	}
	return &s, nil
}
