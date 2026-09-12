-- Normalized out of bookings (3NF): a listing can be blocked for reasons
-- other than a booking (owner maintenance, manual block), so the reason
-- and the optional booking link live in their own table rather than as
-- nullable columns bolted onto gear_listings.
CREATE TABLE availability_blocks (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    listing_id  BIGINT UNSIGNED NOT NULL,
    booking_id  BIGINT UNSIGNED NULL,
    start_date  DATE NOT NULL,
    end_date    DATE NOT NULL,
    reason      ENUM('booked', 'maintenance', 'blocked') NOT NULL DEFAULT 'booked',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_availability_listing FOREIGN KEY (listing_id) REFERENCES gear_listings(id) ON DELETE CASCADE,
    CONSTRAINT fk_availability_booking FOREIGN KEY (booking_id) REFERENCES bookings(id) ON DELETE CASCADE,
    KEY idx_availability_listing_dates (listing_id, start_date, end_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
