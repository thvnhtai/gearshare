package db

import (
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"deadlock (1213) is retryable", &mysql.MySQLError{Number: 1213, Message: "Deadlock found"}, true},
		{"lock wait timeout (1205) is retryable", &mysql.MySQLError{Number: 1205, Message: "Lock wait timeout exceeded"}, true},
		{"duplicate entry (1062) is not retryable", &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}, false},
		{"non-mysql error is not retryable", errNotMySQL{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryable(tt.err); got != tt.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

type errNotMySQL struct{}

func (errNotMySQL) Error() string { return "some other error" }
