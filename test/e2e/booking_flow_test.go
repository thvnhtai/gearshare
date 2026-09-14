//go:build e2e

package e2e_test

// Run with a working Docker daemon:
//   go test -tags=e2e -v ./test/e2e/...
// Spins up a real MySQL container, wires the actual application code
// (the same repositories/services/handlers cmd/api/main.go wires — Redis,
// Kafka, RabbitMQ, and search-indexer are all left unconfigured, which
// every one of those dependencies' nil-safe fallback paths handles by
// design, exactly as it would in a bare `go run ./cmd/api` with no
// docker-compose stack up), and drives the full booking lifecycle over
// real HTTP against an httptest.Server: register -> login -> create
// listing -> create booking -> reject an overlapping attempt -> approve ->
// complete -> leave a review.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/thvnhtai/gearshare/internal/app"
	appmiddleware "github.com/thvnhtai/gearshare/internal/middleware"
	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/availability"
	"github.com/thvnhtai/gearshare/internal/booking"
	"github.com/thvnhtai/gearshare/internal/category"
	"github.com/thvnhtai/gearshare/internal/config"
	gsdb "github.com/thvnhtai/gearshare/internal/db"
	"github.com/thvnhtai/gearshare/internal/listing"
	"github.com/thvnhtai/gearshare/internal/realtime"
	"github.com/thvnhtai/gearshare/internal/review"
	"github.com/thvnhtai/gearshare/internal/user"
)

func TestE2E_FullBookingLifecycle(t *testing.T) {
	ctx := context.Background()

	container, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("gearshare_e2e"),
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("root_password"),
	)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })

	connStr, err := container.ConnectionString(ctx, "parseTime=true&multiStatements=true")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	applyMigrations(t, connStr)

	database, err := gsdb.Connect(ctx, config.MySQLConfig{
		PrimaryDSN: connStr, ReplicaDSN: connStr, MaxOpenConns: 20, MaxIdleConns: 10,
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer database.Close()

	server := buildTestServer(database)
	defer server.Close()

	client := server.Client()

	// --- register owner + renter ---
	ownerToken := registerUser(t, client, server.URL, "owner@e2e.test", "owner")
	renterToken := registerUser(t, client, server.URL, "renter@e2e.test", "renter")

	// --- create listing ---
	listingBody := map[string]any{
		"category_id": 1, "title": "E2E Test Tent", "description": "x",
		"price_per_day_cents": 2000, "deposit_cents": 5000,
	}
	var createdListing struct {
		ID int64 `json:"id"`
	}
	doJSON(t, client, "POST", server.URL+"/api/v1/listings", ownerToken, listingBody, &createdListing)
	if createdListing.ID == 0 {
		t.Fatal("expected a non-zero listing id")
	}

	// --- feed should now include it ---
	var feed []map[string]any
	doJSON(t, client, "GET", server.URL+"/api/v1/listings", "", nil, &feed)
	found := false
	for _, item := range feed {
		if int64(item["listing_id"].(float64)) == createdListing.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected new listing to appear in the feed")
	}

	// --- create booking ---
	start := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	end := time.Now().AddDate(0, 0, 12).Format("2006-01-02")
	var createdBooking struct {
		ID              int64  `json:"id"`
		Status          string `json:"status"`
		TotalPriceCents int64  `json:"total_price_cents"`
	}
	doJSON(t, client, "POST", server.URL+"/api/v1/bookings", renterToken, map[string]any{
		"listing_id": createdListing.ID, "start_date": start, "end_date": end,
	}, &createdBooking)
	// 3 inclusive days (start, start+1, end) at 2000 cents/day — see
	// internal/booking/pricing.go's nightsBetween doc comment.
	if createdBooking.Status != "requested" || createdBooking.TotalPriceCents != 6000 {
		t.Fatalf("unexpected booking state: %+v", createdBooking)
	}

	// --- overlapping booking attempt must be rejected ---
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/bookings", jsonBody(map[string]any{
		"listing_id": createdListing.ID, "start_date": start, "end_date": end,
	}))
	req.Header.Set("Authorization", "Bearer "+renterToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("overlapping booking request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for overlapping booking, got %d", resp.StatusCode)
	}

	// --- owner approves ---
	var approved struct{ Status string `json:"status"` }
	doJSON(t, client, "POST", fmt.Sprintf("%s/api/v1/bookings/%d/approve", server.URL, createdBooking.ID), ownerToken, nil, &approved)
	if approved.Status != "approved" {
		t.Fatalf("expected approved status, got %s", approved.Status)
	}

	// --- owner marks it active then complete (only valid transitions) ---
	if err := database.Primary.QueryRow(`SELECT 1`).Err(); err != nil {
		t.Fatalf("db sanity check: %v", err)
	}
	if _, err := database.Primary.Exec(`UPDATE bookings SET status = 'active' WHERE id = ?`, createdBooking.ID); err != nil {
		t.Fatalf("force-transition to active for test setup: %v", err)
	}
	var completed struct{ Status string `json:"status"` }
	doJSON(t, client, "POST", fmt.Sprintf("%s/api/v1/bookings/%d/complete", server.URL, createdBooking.ID), ownerToken, nil, &completed)
	if completed.Status != "completed" {
		t.Fatalf("expected completed status, got %s", completed.Status)
	}

	// --- renter leaves a review ---
	var createdReview struct{ ID int64 `json:"id"` }
	doJSON(t, client, "POST", server.URL+"/api/v1/reviews", renterToken, map[string]any{
		"booking_id": createdBooking.ID, "rating": 5, "comment": "Great tent!",
	}, &createdReview)
	if createdReview.ID == 0 {
		t.Fatal("expected a non-zero review id")
	}
}

func registerUser(t *testing.T, client *http.Client, base, email, role string) string {
	t.Helper()
	var result struct {
		AccessToken string `json:"access_token"`
	}
	doJSON(t, client, "POST", base+"/api/v1/auth/register", "", map[string]any{
		"email": email, "password": "password123", "display_name": email, "role": role,
	}, &result)
	if result.AccessToken == "" {
		t.Fatalf("expected access token for %s", email)
	}
	return result.AccessToken
}

func jsonBody(v any) *bytes.Reader {
	if v == nil {
		return bytes.NewReader(nil)
	}
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

func doJSON(t *testing.T, client *http.Client, method, url, token string, body any, out any) {
	t.Helper()
	req, err := http.NewRequest(method, url, jsonBody(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("%s %s: unexpected status %d", method, url, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: decode response: %v", method, url, err)
		}
	}
}

// buildTestServer wires the same repositories/services/handlers
// cmd/api/main.go wires for the pieces this e2e flow exercises. Broker/cache
// dependencies (Redis, Kafka, RabbitMQ, search-indexer) are left nil,
// exercising each one's documented no-op/fallback path exactly as a bare
// `go run ./cmd/api` with no docker-compose stack running would.
func buildTestServer(database *gsdb.DB) *httptest.Server {
	userRepo := user.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	listingRepo := listing.NewRepository(database)
	availabilityRepo := availability.NewRepository(database)
	bookingRepo := booking.NewRepository(database)
	reviewRepo := review.NewRepository(database)

	bcryptHasher := auth.NewBcryptHasher(10)
	jwtIssuer := auth.NewJWTIssuer("e2e-test-secret", 15*time.Minute)

	userService := user.NewService(userRepo, bcryptHasher, jwtIssuer)
	feedOptimized := listing.NewFeedServiceOptimized(database)
	listingService := listing.NewService(listingRepo, feedOptimized, categoryRepo, nil)
	bookingTxRunner := booking.NewTxRunner(database, bookingRepo, availabilityRepo, listingRepo)
	bookingService := booking.NewService(bookingRepo, bookingTxRunner, nil, nil, nil, nil)
	reviewService := review.NewService(reviewRepo)

	hub := realtime.NewHub()
	sessionManager := auth.NewSessionManager("e2e-test-session-secret", time.Hour)

	handlers := app.Handlers{
		User:     user.NewHandler(userService),
		Category: category.NewHandler(categoryRepo),
		Listing:  listing.NewHandler(listingService, nil),
		Booking:  booking.NewHandler(bookingService, listingRepo, hub),
		Review:   review.NewHandler(reviewService),
		SSE:      realtime.NewSSEHandler(hub, jwtIssuer),
		WS:       realtime.NewWebSocketHandler(hub, availabilityRepo, []string{"http://localhost"}),
		Polling:  realtime.NewPollingHandler(bookingRepo),
		Search:   func(w http.ResponseWriter, r *http.Request) { http.Error(w, "not configured in e2e test", http.StatusNotImplemented) },
		Admin:    app.NewAdminHandler(sessionManager, userRepo, bcryptHasher, bookingRepo, false),
		Partner:  nil,
	}

	router := app.NewRouter(handlers, app.RouterConfig{
		CORSOrigins:       []string{"http://localhost"},
		JWTIssuer:         jwtIssuer,
		InternalBasicUser: "admin",
		InternalBasicPass: "test",
		Sessions:          sessionManager,
		APIKeyRepo:        auth.NewAPIKeyRepository(database),
		APIKeyManager:     auth.NewAPIKeyManager("e2e-test-pepper"),
		AuthRateLimit:     appmiddleware.RateLimit(20, time.Minute),
		APIRateLimit:      appmiddleware.RateLimit(300, time.Minute),
	})

	return httptest.NewServer(router)
}

func applyMigrations(t *testing.T, dsn string) {
	t.Helper()
	dsnNoParams, _, _ := strings.Cut(dsn, "?")
	dbConn, err := sql.Open("mysql", dsnNoParams+"?multiStatements=true")
	if err != nil {
		t.Fatalf("open for migrations: %v", err)
	}
	defer dbConn.Close()

	migrationsDir := findMigrationsDir(t)
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, f := range files {
		if f == "000001_create_app_user.up.sql" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(migrationsDir, f))
		if err != nil {
			t.Fatalf("read migration %s: %v", f, err)
		}
		if _, err := dbConn.Exec(string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", f, err)
		}
	}
}

func findMigrationsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(root, "migrations", "mysql")
}
