CREATE TABLE gear_listings (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    owner_id            BIGINT UNSIGNED NOT NULL,
    category_id         BIGINT UNSIGNED NOT NULL,
    title               VARCHAR(180) NOT NULL,
    description         TEXT NOT NULL,
    price_per_day_cents INT UNSIGNED NOT NULL,
    deposit_cents       INT UNSIGNED NOT NULL DEFAULT 0,
    status              ENUM('draft', 'active', 'archived') NOT NULL DEFAULT 'draft',
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_listings_owner FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_listings_category FOREIGN KEY (category_id) REFERENCES categories(id),
    KEY idx_listings_owner (owner_id),
    KEY idx_listings_category_status (category_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
