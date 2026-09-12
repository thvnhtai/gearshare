package booking

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ListingCacheInvalidator is satisfied by *cache.ListingCache (structural
// typing, same reasoning as booking.OwnerNotifier in handler.go: this
// package stays free of a direct dependency on internal/cache). Called on
// every status transition that changes a listing's real availability.
type ListingCacheInvalidator interface {
	InvalidateDetail(ctx context.Context, listingID int64)
}

// NotificationQueuer is satisfied by a small adapter around
// internal/queue.Publisher (structural typing, same reasoning as the other
// narrow interfaces in this file): this package publishes an email job to
// RabbitMQ's notifications.email queue on the transitions a renter/owner
// actually needs to hear about, deliberately NOT on every transition (e.g.
// not on the initial "requested" — the renter already knows, they just
// asked for it).
type NotificationQueuer interface {
	QueueEmail(ctx context.Context, payload []byte) error
}

type emailJob struct {
	BookingID int64  `json:"booking_id"`
	RenterID  int64  `json:"renter_id"`
	Template  string `json:"template"`
}

type Service struct {
	repo      *Repository
	txRunner  *TxRunner
	publisher EventPublisher
	cache     ListingCacheInvalidator
	notifier  NotificationQueuer
}

func NewService(repo *Repository, txRunner *TxRunner, publisher EventPublisher, cache ListingCacheInvalidator, notifier NotificationQueuer) *Service {
	return &Service{repo: repo, txRunner: txRunner, publisher: publisher, cache: cache, notifier: notifier}
}

func (s *Service) queueEmail(ctx context.Context, bookingID, renterID int64, template string) {
	if s.notifier == nil {
		return
	}
	payload, err := json.Marshal(emailJob{BookingID: bookingID, RenterID: renterID, Template: template})
	if err != nil {
		return
	}
	if err := s.notifier.QueueEmail(ctx, payload); err != nil {
		fmt.Printf("booking: failed to queue email for booking %d: %v\n", bookingID, err)
	}
}

func (s *Service) invalidateListingCache(ctx context.Context, listingID int64) {
	if s.cache != nil {
		s.cache.InvalidateDetail(ctx, listingID)
	}
}

func (s *Service) CreateBooking(ctx context.Context, listingID, renterID int64, start, end time.Time) (*Booking, error) {
	b, err := s.txRunner.CreateAtomic(ctx, CreateBookingInput{
		ListingID: listingID,
		RenterID:  renterID,
		StartDate: start,
		EndDate:   end,
	})
	if err != nil {
		return nil, err
	}
	s.invalidateListingCache(ctx, b.ListingID)

	if err := publishEvent(ctx, s.publisher, Event{
		Type:       EventBookingCreated,
		BookingID:  b.ID,
		ListingID:  b.ListingID,
		RenterID:   b.RenterID,
		Status:     b.Status,
		OccurredAt: time.Now(),
	}); err != nil {
		// Publish failures never fail the booking itself — the booking is
		// already durably committed in MySQL. This is the "graceful
		// degradation" behavior for the eventing side path; the audit-log
		// and search-indexer consumers will simply lag until the broker
		// recovers, rather than the renter seeing a failed booking.
		fmt.Printf("booking: failed to publish BookingCreated for booking %d: %v\n", b.ID, err)
	}

	return b, nil
}

func (s *Service) Approve(ctx context.Context, bookingID int64) (*Booking, error) {
	return s.transition(ctx, bookingID, StatusApproved, EventBookingApproved)
}

func (s *Service) Reject(ctx context.Context, bookingID int64) (*Booking, error) {
	return s.transition(ctx, bookingID, StatusRejected, EventBookingRejected)
}

func (s *Service) Cancel(ctx context.Context, bookingID int64) (*Booking, error) {
	return s.transition(ctx, bookingID, StatusCancelled, EventBookingCancelled)
}

func (s *Service) Complete(ctx context.Context, bookingID int64) (*Booking, error) {
	return s.transition(ctx, bookingID, StatusCompleted, EventBookingCompleted)
}

func (s *Service) Dispute(ctx context.Context, bookingID int64) (*Booking, error) {
	return s.transition(ctx, bookingID, StatusDisputed, EventBookingDisputed)
}

// emailTemplateFor names which transitions are worth an email — approval
// and disputes are the two moments a renter/owner is actively waiting to
// hear about; rejection/cancellation/completion are surfaced in-app
// (SSE dashboard) only, deliberately, to avoid notification fatigue.
func emailTemplateFor(evtType EventType) (string, bool) {
	switch evtType {
	case EventBookingApproved:
		return "booking_approved", true
	case EventBookingDisputed:
		return "booking_disputed", true
	default:
		return "", false
	}
}

func (s *Service) transition(ctx context.Context, bookingID int64, to Status, evtType EventType) (*Booking, error) {
	if err := s.repo.UpdateStatus(ctx, bookingID, to); err != nil {
		return nil, err
	}
	b, err := s.repo.GetByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	s.invalidateListingCache(ctx, b.ListingID)

	if err := publishEvent(ctx, s.publisher, Event{
		Type:       evtType,
		BookingID:  b.ID,
		ListingID:  b.ListingID,
		RenterID:   b.RenterID,
		Status:     b.Status,
		OccurredAt: time.Now(),
	}); err != nil {
		fmt.Printf("booking: failed to publish %s for booking %d: %v\n", evtType, b.ID, err)
	}

	if template, ok := emailTemplateFor(evtType); ok {
		s.queueEmail(ctx, b.ID, b.RenterID, template)
	}
	return b, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*Booking, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListForOwner(ctx context.Context, ownerID int64) ([]Booking, error) {
	return s.repo.ListForOwner(ctx, ownerID)
}
