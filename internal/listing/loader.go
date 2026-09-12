package listing

import (
	"context"
	"fmt"

	"github.com/thvnhtai/gearshare/internal/category"
	"github.com/thvnhtai/gearshare/internal/review"
	"github.com/thvnhtai/gearshare/internal/user"
)

// FeedServiceBatched is the middle ground between query_naive.go (1+3N
// queries) and query_optimized.go (1 query): collect every ID a page of
// listings references, then issue exactly one batched IN(...) query per
// related entity type — 1 (listings) + 1 (owners) + 1 (categories) +
// 1 (ratings) = 4 queries total, independent of N.
//
// A single JOIN (query_optimized.go) is the better choice when everything
// lives in the same MySQL instance, which is the case here — this type is
// the reference implementation of the *pattern* you reach for when it
// isn't: when the related data lives behind a different service, a
// different database engine (Mongo/Elasticsearch), or a gRPC call, a JOIN
// isn't available and this batch-then-map shape is what replaces it.
type FeedServiceBatched struct {
	listings   *Repository
	users      *user.Repository
	categories *category.Repository
	reviews    *review.Repository
}

func NewFeedServiceBatched(listings *Repository, users *user.Repository, categories *category.Repository, reviews *review.Repository) *FeedServiceBatched {
	return &FeedServiceBatched{listings: listings, users: users, categories: categories, reviews: reviews}
}

func (s *FeedServiceBatched) GetFeed(ctx context.Context, limit, offset int) ([]FeedItem, error) {
	rows, err := s.listings.ListActivePage(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing: batched feed: %w", err)
	}

	ownerIDs := make([]int64, 0, len(rows))
	categoryIDs := make([]int64, 0, len(rows))
	listingIDs := make([]int64, 0, len(rows))
	seenOwner := map[int64]bool{}
	seenCategory := map[int64]bool{}
	for _, l := range rows {
		if !seenOwner[l.OwnerID] {
			seenOwner[l.OwnerID] = true
			ownerIDs = append(ownerIDs, l.OwnerID)
		}
		if !seenCategory[l.CategoryID] {
			seenCategory[l.CategoryID] = true
			categoryIDs = append(categoryIDs, l.CategoryID)
		}
		listingIDs = append(listingIDs, l.ID)
	}

	owners, err := s.users.GetByIDs(ctx, ownerIDs)
	if err != nil {
		return nil, fmt.Errorf("listing: batched feed owners: %w", err)
	}
	cats, err := s.categories.GetByIDs(ctx, categoryIDs)
	if err != nil {
		return nil, fmt.Errorf("listing: batched feed categories: %w", err)
	}
	ratings, err := s.reviews.GetRatingSummaries(ctx, listingIDs)
	if err != nil {
		return nil, fmt.Errorf("listing: batched feed ratings: %w", err)
	}

	items := make([]FeedItem, 0, len(rows))
	for _, l := range rows {
		owner := owners[l.OwnerID]
		cat := cats[l.CategoryID]
		rating := ratings[l.ID] // zero value is fine: 0 reviews, 0 avg

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
