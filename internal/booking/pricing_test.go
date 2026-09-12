package booking

import (
	"testing"
	"time"
)

func day(offset int) time.Time {
	return time.Date(2026, 1, 1+offset, 0, 0, 0, 0, time.UTC)
}

func TestCalculateTotalPriceCents(t *testing.T) {
	tests := []struct {
		name          string
		start, end    time.Time
		pricePerDay   int64
		want          int64
		wantErr       bool
	}{
		{"single day", day(0), day(0), 1000, 1000, false},
		{"three days", day(0), day(2), 1000, 3000, false},
		{"seven days at odd price", day(0), day(6), 1999, 13993, false},
		{"end before start is invalid", day(2), day(0), 1000, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateTotalPriceCents(tt.start, tt.end, tt.pricePerDay)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}
