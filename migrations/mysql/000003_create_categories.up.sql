CREATE TABLE categories (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(80) NOT NULL,
    slug        VARCHAR(80) NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_categories_slug (slug)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO categories (name, slug) VALUES
    ('Camping & Hiking', 'camping-hiking'),
    ('Water Sports', 'water-sports'),
    ('Climbing', 'climbing'),
    ('Cycling', 'cycling'),
    ('Photography', 'photography'),
    ('Winter Sports', 'winter-sports');
