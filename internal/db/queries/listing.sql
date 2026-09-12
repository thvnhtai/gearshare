-- name: ListActiveListingIDs :many
-- Step 1 of the naive feed path (internal/listing/query_naive.go): fetch the
-- page of listing IDs, then loop and issue 3 more queries PER ROW below.
-- This file exists so the naive-vs-optimized comparison in
-- docs/performance/n-plus-one.md has a canonical, reviewable source of truth
-- even though the runtime code (internal/listing/repository.go) is
-- hand-written sqlx pending `make sqlc` — see docs/adr/0001-orm-choice.md.
SELECT id, owner_id, category_id, title, description, price_per_day_cents, deposit_cents, status, created_at, updated_at
FROM gear_listings
WHERE status = 'active'
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: GetOwnerByID :one
-- N+1 query #2 (naive path): one call per listing to fetch its owner.
SELECT id, display_name, email FROM users WHERE id = ?;

-- name: GetCategoryByID :one
-- N+1 query #3 (naive path): one call per listing to fetch its category.
SELECT id, name, slug FROM categories WHERE id = ?;

-- name: GetListingRatingSummary :one
-- N+1 query #4 (naive path): one call per listing to aggregate its reviews.
SELECT COUNT(*) AS review_count, COALESCE(AVG(r.rating), 0) AS avg_rating
FROM reviews r
JOIN bookings b ON b.id = r.booking_id
WHERE b.listing_id = ?;

-- name: ListActiveListingsFeedOptimized :many
-- The fix: ONE query total. Owner and category are pulled in via JOIN;
-- rating/review-count are pulled in via a correlated subquery that MySQL
-- executes once per listing *inside a single query plan* (see the EXPLAIN
-- comparison in docs/performance/n-plus-one.md), not as N separate
-- round-trips from the application.
SELECT
    gl.id, gl.title, gl.price_per_day_cents, gl.deposit_cents, gl.status, gl.created_at,
    u.id AS owner_id, u.display_name AS owner_name,
    c.id AS category_id, c.name AS category_name, c.slug AS category_slug,
    COALESCE(rs.review_count, 0) AS review_count,
    COALESCE(rs.avg_rating, 0) AS avg_rating
FROM gear_listings gl
JOIN users u ON u.id = gl.owner_id
JOIN categories c ON c.id = gl.category_id
LEFT JOIN (
    SELECT b.listing_id, COUNT(*) AS review_count, AVG(r.rating) AS avg_rating
    FROM reviews r
    JOIN bookings b ON b.id = r.booking_id
    GROUP BY b.listing_id
) rs ON rs.listing_id = gl.id
WHERE gl.status = 'active'
ORDER BY gl.created_at DESC
LIMIT ? OFFSET ?;
