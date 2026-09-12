CREATE TABLE bookings (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    listing_id          BIGINT UNSIGNED NOT NULL,
    renter_id           BIGINT UNSIGNED NOT NULL,
    start_date          DATE NOT NULL,
    end_date            DATE NOT NULL,
    status              ENUM('requested', 'approved', 'rejected', 'active', 'completed', 'cancelled', 'disputed')
                            NOT NULL DEFAULT 'requested',
    total_price_cents   INT UNSIGNED NOT NULL,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_bookings_listing FOREIGN KEY (listing_id) REFERENCES gear_listings(id) ON DELETE CASCADE,
    CONSTRAINT fk_bookings_renter FOREIGN KEY (renter_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chk_bookings_dates CHECK (end_date >= start_date),
    -- Backs the "listing feed" N+1 fix and the scaling-pattern index-tuning
    -- writeup in docs/performance/n-plus-one.md.
    KEY idx_bookings_listing_status_start (listing_id, status, start_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
