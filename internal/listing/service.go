package listing

import (
	"context"
	"errors"
	"fmt"

	"github.com/thvnhtai/gearshare/internal/category"
)

var ErrInvalidInput = errors.New("listing: invalid input")

type Service struct {
	repo       *Repository
	feed       *FeedServiceOptimized
	categories *category.Repository
	publisher  EventPublisher
}

func NewService(repo *Repository, feed *FeedServiceOptimized, categories *category.Repository, publisher EventPublisher) *Service {
	return &Service{repo: repo, feed: feed, categories: categories, publisher: publisher}
}

type CreateInput struct {
	OwnerID          int64
	CategoryID       int64
	Title            string
	Description      string
	PricePerDayCents int64
	DepositCents     int64
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*Listing, error) {
	if in.Title == "" || in.PricePerDayCents <= 0 {
		return nil, ErrInvalidInput
	}

	l := &Listing{
		OwnerID:          in.OwnerID,
		CategoryID:       in.CategoryID,
		Title:            in.Title,
		Description:      in.Description,
		PricePerDayCents: in.PricePerDayCents,
		DepositCents:     in.DepositCents,
		Status:           StatusActive,
	}
	id, err := s.repo.Create(ctx, l)
	if err != nil {
		return nil, err
	}
	// Re-fetch rather than trust the in-memory struct: created_at/updated_at
	// are set by the database (DEFAULT CURRENT_TIMESTAMP), not by this code.
	created, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// The event carries the full listing content — see events.go's doc
	// comment on why (search-indexer has no MySQL access of its own).
	categoryName := ""
	if cat, err := s.categories.GetByID(ctx, created.CategoryID); err == nil {
		categoryName = cat.Name
	}
	if err := publishEvent(ctx, s.publisher, Event{
		Type:             EventListingCreated,
		ListingID:        created.ID,
		Title:            created.Title,
		Description:      created.Description,
		PricePerDayCents: created.PricePerDayCents,
		CategoryName:     categoryName,
		Status:           created.Status,
	}); err != nil {
		fmt.Printf("listing: failed to publish ListingCreated for listing %d: %v\n", created.ID, err)
	}

	return created, nil
}

// Feed always calls the optimized (JOIN-based) path — see handler.go and
// query_naive.go's doc comment for why the naive path is a benchmark
// fixture, not something the API ever serves traffic through.
func (s *Service) Feed(ctx context.Context, limit, offset int) ([]FeedItem, error) {
	return s.feed.GetFeed(ctx, limit, offset)
}

func (s *Service) GetDetail(ctx context.Context, id int64) (*FeedItem, error) {
	return s.feed.GetByID(ctx, id)
}
