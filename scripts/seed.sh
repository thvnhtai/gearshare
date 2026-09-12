#!/usr/bin/env bash
# Seeds a demo owner, renter, and a handful of listings against a running
# API (default: http://localhost:8080). Convenient for manually exploring
# web/ or re-populating a fresh docker-compose stack.
set -euo pipefail

BASE="${GEARSHARE_API_BASE:-http://localhost:8080}"

echo "Registering demo owner..."
OWNER_TOKEN=$(curl -s -X POST "$BASE/api/v1/auth/register" -H 'Content-Type: application/json' \
  -d '{"email":"demo-owner@gearshare.test","password":"password123","display_name":"Demo Owner","role":"owner"}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')

echo "Registering demo renter..."
curl -s -X POST "$BASE/api/v1/auth/register" -H 'Content-Type: application/json' \
  -d '{"email":"demo-renter@gearshare.test","password":"password123","display_name":"Demo Renter"}' > /dev/null

echo "Creating demo listings..."
curl -s -X POST "$BASE/api/v1/listings" -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"category_id":1,"title":"4-Person Tent","description":"Great for weekend trips","price_per_day_cents":1500,"deposit_cents":5000}' > /dev/null
curl -s -X POST "$BASE/api/v1/listings" -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"category_id":2,"title":"Touring Kayak","description":"Single-seat, stable in flat water","price_per_day_cents":3000,"deposit_cents":10000}' > /dev/null
curl -s -X POST "$BASE/api/v1/listings" -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"category_id":5,"title":"Mirrorless Camera Kit","description":"Body + two lenses","price_per_day_cents":4500,"deposit_cents":30000}' > /dev/null

echo "Done. demo-owner@gearshare.test / demo-renter@gearshare.test, password: password123"
