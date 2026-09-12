# MongoDB collections

GearShare uses MongoDB for exactly two collections — both deliberately kept
out of MySQL because their shape is naturally document-like, not
relational. See [`docs/erd/gearshare-erd.md`](../../docs/erd/gearshare-erd.md)
for why forcing either into MySQL would break 3NF.

## `listing_specs`

One document per listing, keyed by `listing_id`. Attributes vary
completely by category — a tent's spec sheet (capacity, season rating,
packed weight) shares nothing with a camera's (megapixels, lens mount).

```json
{
  "listing_id": 1,
  "category": "camping-hiking",
  "attributes": {
    "capacity_persons": 4,
    "season_rating": "3-season",
    "packed_weight_kg": 3.2,
    "waterproof_rating_mm": 3000
  },
  "updated_at": "2026-01-15T10:00:00Z"
}
```

Managed by [`internal/spec`](../../internal/spec).

## `damage_reports`

One document per incident report, filed from `POST /api/v1/bookings/{id}/dispute`.
Freeform description plus a variable-length photo array — not something a
fixed relational schema models comfortably.

```json
{
  "_id": "...",
  "booking_id": 42,
  "reported_by": 7,
  "description": "Tent pole snapped during setup, otherwise fine.",
  "photo_urls": ["https://.../photo1.jpg", "https://.../photo2.jpg"],
  "created_at": "2026-02-01T08:30:00Z"
}
```

Managed by [`internal/damagereport`](../../internal/damagereport).

See `seed_listing_specs.json` / `seed_damage_reports.json` in this
directory for loadable examples (`mongoimport --db gearshare --collection
listing_specs --file seed_listing_specs.json --jsonArray`).
