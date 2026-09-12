package review

import (
	"context"
	"errors"
)

var ErrInvalidRating = errors.New("review: rating must be between 1 and 5")

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, bookingID, reviewerID int64, rating int, comment string) (*Review, error) {
	if rating < 1 || rating > 5 {
		return nil, ErrInvalidRating
	}

	r := &Review{BookingID: bookingID, ReviewerID: reviewerID, Rating: rating, Comment: comment}
	id, err := s.repo.Create(ctx, r)
	if err != nil {
		return nil, err
	}
	r.ID = id
	return r, nil
}
