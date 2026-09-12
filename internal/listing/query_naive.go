package listing

import (
	"context"
	"fmt"

	"github.com/thvnhtai/gearshare/internal/category"
	"github.com/thvnhtai/gearshare/internal/review"
	"github.com/thvnhtai/gearshare/internal/user"
)

// FeedServiceNaive assembles the browse/search feed the way it's easy to
// write by accident: fetch the page of listings, then loop and fetch each
// listing's owner, category, and rating summary one at a time.
//
// For a page of N listings this issues 1 + 3N round trips to MySQL. At
// N=20 that's 61 queries for one page load. See query_optimized.go for the
// JOIN-based fix and loader.go for the batched-dataloader fix, and
// docs/performance/n-plus-one.md for the measured difference
// (feed_bench_test.go) and EXPLAIN output.
//
// This type exists to be benchmarked against, not to be wired into the real
// GET /api/v1/listings handler — see internal/listing/handler.go, which
// calls FeedServiceOptimized.
type FeedServiceNaive struct {
	listings   *Repository
	users      *user.Repository
	categories *category.Repository
	reviews    *review.Repository
}

func NewFeedServiceNaive(listings *Repository, users *user.Repository, categories *category.Repository, reviews *review.Repository) *FeedServiceNaive {
	return &FeedServiceNaive{listings: listings, users: users, categories: categories, reviews: reviews}
}

func (s *FeedServiceNaive) GetFeed(ctx context.Context, limit, offset int) ([]FeedItem, error) {
	// Query 1: the page of listings.
	rows, err := s.listings.ListActivePage(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing: naive feed: %w", err)
	}

	items := make([]FeedItem, 0, len(rows))
	for _, l := range rows {
		// Query 2 (per row): owner.
		owner, err := s.users.GetByID(ctx, l.OwnerID)
		if err != nil {
			return nil, fmt.Errorf("listing: naive feed owner lookup: %w", err)
		}
		// Query 3 (per row): category.
		cat, err := s.categories.GetByID(ctx, l.CategoryID)
		if err != nil {
			return nil, fmt.Errorf("listing: naive feed category lookup: %w", err)
		}
		// Query 4 (per row): rating aggregate.
		rating, err := s.reviews.GetRatingSummary(ctx, l.ID)
		if err != nil {
			return nil, fmt.Errorf("listing: naive feed rating lookup: %w", err)
		}

		items = append(items, FeedItem{
			ListingID:        l.ID,
			Title:            l.Title,
			PricePerDayCents: l.PricePerDayCents,
			DepositCents:     l.DepositCents,
			OwnerID:          owner.ID,
			OwnerName:        owner.DisplayName,
			CategoryID:       cat.ID,
			CategoryName:     cat.Name,
			ReviewCount:      rating.ReviewCount,
			AvgRating:        rating.AvgRating,
		})
	}
	return items, nil
}
