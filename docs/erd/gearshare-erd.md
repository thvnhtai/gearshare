# Entity-relationship diagram (MySQL schema)

```mermaid
erDiagram
    USERS ||--o{ OAUTH_IDENTITIES : "has"
    USERS ||--o{ REFRESH_TOKENS : "has"
    USERS ||--o{ GEAR_LISTINGS : "owns"
    USERS ||--o{ BOOKINGS : "rents as renter"
    USERS ||--o{ REVIEWS : "writes"
    CATEGORIES ||--o{ GEAR_LISTINGS : "classifies"
    GEAR_LISTINGS ||--o{ BOOKINGS : "is booked via"
    GEAR_LISTINGS ||--o{ AVAILABILITY_BLOCKS : "has calendar of"
    BOOKINGS ||--o| AVAILABILITY_BLOCKS : "creates"
    BOOKINGS ||--o| REVIEWS : "is reviewed via"

    USERS {
        bigint id PK
        varchar email UK
        varchar password_hash
        enum password_algo
        varchar display_name
        enum role
    }
    OAUTH_IDENTITIES {
        bigint id PK
        bigint user_id FK
        enum provider
        varchar provider_user_id
    }
    REFRESH_TOKENS {
        bigint id PK
        bigint user_id FK
        char_64 token_hash UK
        timestamp expires_at
    }
    API_KEYS {
        bigint id PK
        varchar owner_label
        char_64 key_hash UK
    }
    CATEGORIES {
        bigint id PK
        varchar name
        varchar slug UK
    }
    GEAR_LISTINGS {
        bigint id PK
        bigint owner_id FK
        bigint category_id FK
        varchar title
        int price_per_day_cents
        int deposit_cents
        enum status
    }
    BOOKINGS {
        bigint id PK
        bigint listing_id FK
        bigint renter_id FK
        date start_date
        date end_date
        enum status
        int total_price_cents
    }
    AVAILABILITY_BLOCKS {
        bigint id PK
        bigint listing_id FK
        bigint booking_id FK "nullable"
        date start_date
        date end_date
        enum reason
    }
    REVIEWS {
        bigint id PK
        bigint booking_id FK UK
        bigint reviewer_id FK
        tinyint rating
    }
```

## Why this is 3NF, not just "a schema that works"

- **1NF**: every column holds a single atomic value — no CSV-in-a-varchar
  category lists, no JSON blobs standing in for a proper relationship.
  (`listing_specs`, which genuinely is variable-shaped per category, was
  deliberately moved *out* of MySQL into MongoDB rather than modeled as a
  1NF violation — see `docs/architecture.md`'s MySQL-vs-MongoDB split.)
- **2NF**: every non-key column depends on the *whole* primary key. This
  only becomes a real constraint on composite keys, and this schema has
  none — every table uses a single surrogate `id` — so 2NF is satisfied
  trivially, which is itself a deliberate design choice (surrogate keys
  over composite natural keys) rather than an accident.
- **3NF**: every non-key column depends on the key, the whole key, and
  nothing but the key. The concrete case this schema had to get right:
  `availability_blocks` is its own table rather than nullable
  `blocked_from`/`blocked_to`/`block_reason` columns bolted onto
  `gear_listings`, because a listing can have *many* blocks (multiple
  bookings, a maintenance window, a manual block) — cramming that into the
  listing row would mean `gear_listings.blocked_from` transitively depends
  on "which block", not on the listing's own key. Likewise, `reviews`
  stores `booking_id` (which transitively determines `listing_id` via
  `bookings.listing_id`) rather than a redundant `listing_id` column —
  the N+1 fix's JOIN through `bookings` (see
  `docs/performance/n-plus-one.md`) is the direct consequence of keeping
  that relationship normalized instead of denormalizing it for query
  convenience.

## Deliberate denormalization: none, by default
No column in this schema is a cached/duplicated copy of another table's
data. The Redis cache-aside layer (`internal/cache/listing_cache.go`) is
where GearShare trades consistency for read speed — at the cache layer,
with an explicit invalidation path, not by denormalizing the relational
schema itself and accepting silent staleness there.
