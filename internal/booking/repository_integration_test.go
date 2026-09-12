//go:build integration

package booking_test

// Run with a working Docker daemon:
//   go test -tags=integration -run TestIntegration -v ./internal/booking/...
// Spins up a real MySQL 8 container (testcontainers-go), applies the
// project's migrations, then races two concurrent CreateAtomic calls for
// the SAME listing and overlapping dates — the actual failure mode this
// project's transaction design (internal/booking/tx.go, internal/db/tx.go)
// exists to prevent. Demonstrates two things end to end, not just in
// isolation: (1) exactly one of the two concurrent bookings wins, the
// other gets ErrListingUnavailable, and (2) there is never a partial write
// — every booking row has exactly one matching availability_blocks row,
// checked directly against the database after the race.

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/thvnhtai/gearshare/internal/availability"
	"github.com/thvnhtai/gearshare/internal/booking"
	"github.com/thvnhtai/gearshare/internal/config"
	gsdb "github.com/thvnhtai/gearshare/internal/db"
	"github.com/thvnhtai/gearshare/internal/listing"
)

func TestIntegration_ConcurrentOverlappingBookings_ExactlyOneWins(t *testing.T) {
	ctx := context.Background()

	container, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("gearshare_test"),
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("root_password"),
	)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

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

	ownerID := insertUser(t, database, "owner@race.test", "owner")
	renterAID := insertUser(t, database, "renter-a@race.test", "renter")
	renterBID := insertUser(t, database, "renter-b@race.test", "renter")
	listingID := insertListing(t, database, ownerID)

	listingRepo := listing.NewRepository(database)
	availabilityRepo := availability.NewRepository(database)
	bookingRepo := booking.NewRepository(database)
	txRunner := booking.NewTxRunner(database, bookingRepo, availabilityRepo, listingRepo)

	start := time.Now().AddDate(0, 0, 30)
	end := start.AddDate(0, 0, 2)

	var wg sync.WaitGroup
	results := make([]error, 2)
	renters := []int64{renterAID, renterBID}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := txRunner.CreateAtomic(ctx, booking.CreateBookingInput{
				ListingID: listingID, RenterID: renters[i], StartDate: start, EndDate: end,
			})
			results[i] = err
		}(i)
	}
	wg.Wait()

	successCount := 0
	conflictCount := 0
	for _, err := range results {
		switch {
		case err == nil:
			successCount++
		case err == booking.ErrListingUnavailable:
			conflictCount++
		default:
			t.Fatalf("unexpected error from concurrent booking attempt: %v", err)
		}
	}
	if successCount != 1 || conflictCount != 1 {
		t.Fatalf("expected exactly 1 success and 1 conflict, got %d successes and %d conflicts", successCount, conflictCount)
	}

	// No partial writes: every booking for this listing has exactly one
	// availability_blocks row, and vice versa.
	var bookingCount, blockCount int
	if err := database.Primary.Get(&bookingCount, `SELECT COUNT(*) FROM bookings WHERE listing_id = ?`, listingID); err != nil {
		t.Fatalf("count bookings: %v", err)
	}
	if err := database.Primary.Get(&blockCount, `SELECT COUNT(*) FROM availability_blocks WHERE listing_id = ?`, listingID); err != nil {
		t.Fatalf("count blocks: %v", err)
	}
	if bookingCount != 1 || blockCount != 1 {
		t.Fatalf("expected exactly 1 booking and 1 availability block, got %d bookings and %d blocks (partial write detected)", bookingCount, blockCount)
	}
}

func insertUser(t *testing.T, database *gsdb.DB, email, role string) int64 {
	t.Helper()
	res, err := database.Primary.Exec(
		`INSERT INTO users (email, password_hash, password_algo, display_name, role) VALUES (?, 'x', 'bcrypt', ?, ?)`,
		email, email, role)
	if err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func insertListing(t *testing.T, database *gsdb.DB, ownerID int64) int64 {
	t.Helper()
	var categoryID int64
	if err := database.Primary.Get(&categoryID, `SELECT id FROM categories LIMIT 1`); err != nil {
		t.Fatalf("lookup category: %v", err)
	}
	res, err := database.Primary.Exec(
		`INSERT INTO gear_listings (owner_id, category_id, title, description, price_per_day_cents, deposit_cents, status)
		 VALUES (?, ?, 'Race Test Listing', 'x', 1000, 0, 'active')`, ownerID, categoryID)
	if err != nil {
		t.Fatalf("insert listing: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// applyMigrations runs every migrations/mysql/*.up.sql file directly
// against the test container — avoids adding a golang-migrate CLI
// dependency to the test binary just to run a handful of files once.
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
			continue // creates a separate MySQL user with a fixed password; irrelevant when the test connects as root
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
	// this file lives at internal/booking/repository_integration_test.go
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(root, "migrations", "mysql")
}
