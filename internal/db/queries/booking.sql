-- name: CreateBooking :execlastid
-- Executed inside internal/booking/tx.go's sql.Tx alongside InsertAvailabilityBlock.
INSERT INTO bookings (listing_id, renter_id, start_date, end_date, status, total_price_cents)
VALUES (?, ?, ?, ?, 'requested', ?);

-- name: InsertAvailabilityBlock :execlastid
INSERT INTO availability_blocks (listing_id, booking_id, start_date, end_date, reason)
VALUES (?, ?, ?, ?, 'booked');

-- name: CountOverlappingBlocks :one
-- Guards against double-booking; run inside the same transaction as
-- CreateBooking so the check-then-insert is atomic under row locks.
SELECT COUNT(*) FROM availability_blocks
WHERE listing_id = ?
  AND start_date <= ?
  AND end_date >= ?;

-- name: UpdateBookingStatus :exec
UPDATE bookings SET status = ? WHERE id = ?;

-- name: GetBookingByID :one
SELECT id, listing_id, renter_id, start_date, end_date, status, total_price_cents, created_at, updated_at
FROM bookings WHERE id = ?;

-- name: ListBookingsForOwner :many
-- Backs the SSE owner-dashboard feed (internal/realtime/sse.go): polled by
-- the hub on an interval and diffed against the last-seen state.
SELECT b.id, b.listing_id, b.renter_id, b.start_date, b.end_date, b.status, b.total_price_cents, b.updated_at
FROM bookings b
JOIN gear_listings gl ON gl.id = b.listing_id
WHERE gl.owner_id = ?
ORDER BY b.updated_at DESC;
