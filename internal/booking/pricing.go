package booking

import (
	"errors"
	"time"
)

var ErrInvalidDateRange = errors.New("booking: end date must be on or after start date")

// nightsBetween counts inclusive rental days: booking July 1-1 is 1 day,
// July 1-3 is 3 days. Pure function, no I/O — exercised directly by
// pricing_test.go (the "unit tests: pure pricing/availability logic"
// requirement) without spinning up a database.
func nightsBetween(start, end time.Time) int {
	days := int(end.Sub(start).Hours()/24) + 1
	if days < 1 {
		return 0
	}
	return days
}

// CalculateTotalPriceCents is the pure pricing calculation used by
// Service.CreateBooking before anything touches the database.
func CalculateTotalPriceCents(start, end time.Time, pricePerDayCents int64) (int64, error) {
	if end.Before(start) {
		return 0, ErrInvalidDateRange
	}
	days := nightsBetween(start, end)
	return int64(days) * pricePerDayCents, nil
}

// DatesOverlap is the pure predicate behind the DB-level overlap check in
// internal/availability/repository.go's CountOverlapping — kept here too so
// the service layer can fail fast on an obviously-invalid range (end before
// start, zero-length) without a round trip, and so the overlap logic itself
// has a unit test independent of MySQL.
func DatesOverlap(aStart, aEnd, bStart, bEnd time.Time) bool {
	return !aEnd.Before(bStart) && !bEnd.Before(aStart)
}
