package absences

import (
	"testing"
	"time"
)

func TestValidation(t *testing.T) {
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, start, end, key string
		bad                   bool
	}{
		{"valid", "2026-11-13", "2026-11-14", "a", false},
		{"past", "2026-11-12", "2026-11-14", "a", true},
		{"reversed", "2026-11-15", "2026-11-14", "a", true},
		{"invalid", "2026-02-30", "2026-11-14", "a", true},
		{"missing key", "2026-11-13", "2026-11-14", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(Input{StartDate: tc.start, EndDate: tc.end, IdempotencyKey: tc.key}, now)
			if (err != nil) != tc.bad {
				t.Fatal(err)
			}
		})
	}
}
