package booking

import "testing"

func TestDatesOverlap(t *testing.T) {
	tests := []struct {
		name                   string
		aStart, aEnd           int
		bStart, bEnd           int
		want                   bool
	}{
		{"identical ranges overlap", 0, 5, 0, 5, true},
		{"a fully inside b", 2, 3, 0, 5, true},
		{"adjacent, touching end/start counts as overlap (inclusive dates)", 0, 2, 2, 4, true},
		{"disjoint, gap between", 0, 2, 5, 7, false},
		{"b entirely before a", 5, 7, 0, 2, false},
		{"a entirely before b", 0, 2, 5, 7, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DatesOverlap(day(tt.aStart), day(tt.aEnd), day(tt.bStart), day(tt.bEnd))
			if got != tt.want {
				t.Errorf("DatesOverlap(%d-%d, %d-%d) = %v, want %v",
					tt.aStart, tt.aEnd, tt.bStart, tt.bEnd, got, tt.want)
			}
		})
	}
}

func TestCanTransition(t *testing.T) {
	valid := []struct{ from, to Status }{
		{StatusRequested, StatusApproved},
		{StatusRequested, StatusRejected},
		{StatusApproved, StatusActive},
		{StatusActive, StatusCompleted},
		{StatusActive, StatusDisputed},
	}
	for _, tc := range valid {
		if !CanTransition(tc.from, tc.to) {
			t.Errorf("expected %s -> %s to be valid", tc.from, tc.to)
		}
	}

	invalid := []struct{ from, to Status }{
		{StatusRequested, StatusCompleted},
		{StatusCompleted, StatusActive},
		{StatusRejected, StatusApproved},
	}
	for _, tc := range invalid {
		if CanTransition(tc.from, tc.to) {
			t.Errorf("expected %s -> %s to be invalid", tc.from, tc.to)
		}
	}
}
