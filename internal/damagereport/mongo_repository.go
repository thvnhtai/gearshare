package damagereport

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const collectionName = "damage_reports"

type Repository struct {
	collection *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{collection: db.Collection(collectionName)}
}

func (r *Repository) Create(ctx context.Context, report *Report) (string, error) {
	report.CreatedAt = time.Now()
	res, err := r.collection.InsertOne(ctx, report)
	if err != nil {
		return "", fmt.Errorf("damagereport: create: %w", err)
	}
	id, ok := res.InsertedID.(primitive.ObjectID)
	if !ok {
		return "", fmt.Errorf("damagereport: unexpected inserted id type")
	}
	return id.Hex(), nil
}

func (r *Repository) ListForBooking(ctx context.Context, bookingID int64) ([]Report, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"booking_id": bookingID})
	if err != nil {
		return nil, fmt.Errorf("damagereport: list: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	var reports []Report
	if err := cursor.All(ctx, &reports); err != nil {
		return nil, fmt.Errorf("damagereport: decode: %w", err)
	}
	return reports, nil
}
