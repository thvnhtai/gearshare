package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/thvnhtai/gearshare/internal/availability"
	"github.com/thvnhtai/gearshare/internal/db"
	"github.com/thvnhtai/gearshare/internal/listing"
)

var ErrListingUnavailable = errors.New("booking: listing not available for the requested dates")

// CreateAtomic is the core transactional path required by the "database
// depth: transactions" checklist item: the booking row and its
// availability_blocks row are written inside ONE sql.Tx, with a row-locked
// overlap check in between, so two concurrent requests for the same dates
// can never both succeed. If the overlap check or either insert fails, the
// whole transaction rolls back — no orphaned booking-without-a-block or
// block-without-a-booking is ever visible to another connection.
//
// database.WithinTransaction (internal/db/tx.go) wraps this with
// deadlock/lock-wait-timeout retry, which is GearShare's documented failure
// mode for this specific hot path (docs/adr/0002-cap-tradeoffs.md).
type TxRunner struct {
	database     *db.DB
	bookings     *Repository
	availability *availability.Repository
	listings     *listing.Repository
}

func NewTxRunner(database *db.DB, bookings *Repository, availabilityRepo *availability.Repository, listings *listing.Repository) *TxRunner {
	return &TxRunner{database: database, bookings: bookings, availability: availabilityRepo, listings: listings}
}

type CreateBookingInput struct {
	ListingID int64
	RenterID  int64
	StartDate time.Time
	EndDate   time.Time
}

func (t *TxRunner) CreateAtomic(ctx context.Context, in CreateBookingInput) (*Booking, error) {
	l, err := t.listings.GetByID(ctx, in.ListingID)
	if err != nil {
		return nil, fmt.Errorf("booking: load listing: %w", err)
	}

	totalPrice, err := CalculateTotalPriceCents(in.StartDate, in.EndDate, l.PricePerDayCents)
	if err != nil {
		return nil, err
	}

	var created *Booking
	err = t.database.WithinTransaction(ctx, func(tx *sqlx.Tx) error {
		// Row-locks any existing blocks for this listing that overlap the
		// requested range, so a concurrent transaction attempting the same
		// dates blocks here until we commit or roll back.
		overlapping, err := t.availability.CountOverlapping(ctx, tx, in.ListingID, in.StartDate, in.EndDate)
		if err != nil {
			return err
		}
		if overlapping > 0 {
			return ErrListingUnavailable
		}

		b := &Booking{
			ListingID:       in.ListingID,
			RenterID:        in.RenterID,
			StartDate:       in.StartDate,
			EndDate:         in.EndDate,
			Status:          StatusRequested,
			TotalPriceCents: totalPrice,
		}
		bookingID, err := t.bookings.CreateInTx(ctx, tx, b)
		if err != nil {
			return err
		}
		b.ID = bookingID

		if _, err := t.availability.InsertBlock(ctx, tx, in.ListingID, bookingID, in.StartDate, in.EndDate, availability.ReasonBooked); err != nil {
			return err
		}

		created = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Re-fetch rather than trust the in-memory struct: created_at/updated_at
	// are set by the database (DEFAULT CURRENT_TIMESTAMP), not by this code.
	return t.bookings.GetByID(ctx, created.ID)
}
