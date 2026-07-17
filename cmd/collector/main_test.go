package main

import (
	"testing"
	"time"
)

func TestDurationUntilNextGeoIPReload(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Duration
	}{
		{
			name: "middle of day",
			now:  time.Date(2026, 7, 17, 12, 30, 0, 0, time.UTC),
			want: 11*time.Hour + 30*time.Minute,
		},
		{
			name: "exact midnight",
			now:  time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC),
			want: 24 * time.Hour,
		},
		{
			name: "converts to UTC",
			now:  time.Date(2026, 7, 17, 23, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60)),
			want: 3 * time.Hour,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := durationUntilNextGeoIPReload(test.now); got != test.want {
				t.Fatalf("unexpected delay: got %s want %s", got, test.want)
			}
		})
	}
}
