//go:build integration

package listing_test

// Run with the isolated test stack: `make compose-test-up` then
//   go test -tags=integration -bench=BenchmarkFeed -benchtime=5x ./internal/listing/...
// Results feed docs/performance/n-plus-one.md. Skips automatically if
// MYSQL_TEST_DSN isn't set (e.g. plain `go test ./...` in CI without the
// docker-compose.test.yml stack running).

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/thvnhtai/gearshare/internal/category"
	"github.com/thvnhtai/gearshare/internal/config"
	gsdb "github.com/thvnhtai/gearshare/internal/db"
	"github.com/thvnhtai/gearshare/internal/listing"
	"github.com/thvnhtai/gearshare/internal/review"
	"github.com/thvnhtai/gearshare/internal/user"
)

const seedListingCount = 200

func setupBenchDB(tb testing.TB) *gsdb.DB {
	tb.Helper()
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		tb.Skip("MYSQL_TEST_DSN not set; run `make compose-test-up` and export it, e.g. " +
			"root:root_password@tcp(127.0.0.1:3406)/gearshare_test?parseTime=true&multiStatements=true")
	}

	database, err := gsdb.Connect(context.Background(), config.MySQLConfig{
		PrimaryDSN:   dsn,
		ReplicaDSN:   dsn,
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	})
	if err != nil {
		tb.Fatalf("connect: %v", err)
	}
	seedFeedFixtures(tb, database)
	return database
}

func seedFeedFixtures(tb testing.TB, database *gsdb.DB) {
	tb.Helper()
	ctx := context.Background()

	res, err := database.Primary.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, password_algo, display_name, role) VALUES (?, 'x', 'bcrypt', 'Bench Owner', 'owner')`,
		fmt.Sprintf("bench-owner-%d-%d@example.com", os.Getpid(), time.Now().UnixNano()))
	if err != nil {
		tb.Fatalf("seed owner: %v", err)
	}
	ownerID, err := res.LastInsertId()
	if err != nil {
		tb.Fatalf("seed owner id: %v", err)
	}

	var categoryID int64
	if err := database.Primary.QueryRowContext(ctx, `SELECT id FROM categories LIMIT 1`).Scan(&categoryID); err != nil {
		tb.Fatalf("seed category lookup: %v", err)
	}

	for i := 0; i < seedListingCount; i++ {
		if _, err := database.Primary.ExecContext(ctx,
			`INSERT INTO gear_listings (owner_id, category_id, title, description, price_per_day_cents, deposit_cents, status)
			 VALUES (?, ?, ?, 'bench fixture', 1000, 5000, 'active')`,
			ownerID, categoryID, fmt.Sprintf("Bench listing %d", i)); err != nil {
			tb.Fatalf("seed listing %d: %v", i, err)
		}
	}
}

func BenchmarkFeedNaive(b *testing.B) {
	database := setupBenchDB(b)
	svc := listing.NewFeedServiceNaive(
		listing.NewRepository(database),
		user.NewRepository(database),
		category.NewRepository(database),
		review.NewRepository(database),
	)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.GetFeed(context.Background(), 20, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFeedBatched(b *testing.B) {
	database := setupBenchDB(b)
	svc := listing.NewFeedServiceBatched(
		listing.NewRepository(database),
		user.NewRepository(database),
		category.NewRepository(database),
		review.NewRepository(database),
	)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.GetFeed(context.Background(), 20, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFeedOptimizedJoin(b *testing.B) {
	database := setupBenchDB(b)
	svc := listing.NewFeedServiceOptimized(database)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.GetFeed(context.Background(), 20, 0); err != nil {
			b.Fatal(err)
		}
	}
}
