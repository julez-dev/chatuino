package mainui

import (
	"testing"
	"time"
)

func Test_humanizeDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{name: "zero", duration: 0, want: "0 seconds"},
		{name: "sub-second", duration: 500 * time.Millisecond, want: "0 seconds"},
		{name: "1 second", duration: time.Second, want: "1 second"},
		{name: "30 seconds", duration: 30 * time.Second, want: "30 seconds"},
		{name: "1 minute", duration: time.Minute, want: "1 minute"},
		{name: "1 minute 30 seconds", duration: 90 * time.Second, want: "1 minute 30 seconds"},
		{name: "5 minutes", duration: 5 * time.Minute, want: "5 minutes"},
		{name: "1 hour", duration: time.Hour, want: "1 hour"},
		{name: "1 hour 30 minutes", duration: 90 * time.Minute, want: "1 hour 30 minutes"},
		{name: "2 hours", duration: 2 * time.Hour, want: "2 hours"},
		{name: "1 day", duration: 24 * time.Hour, want: "1 day"},
		{name: "1 day 1 hour", duration: 25 * time.Hour, want: "1 day 1 hour"},
		{name: "1 day 2 hours", duration: 26 * time.Hour, want: "1 day 2 hours"},
		{name: "2 days", duration: 48 * time.Hour, want: "2 days"},
		{name: "7 days", duration: 7 * 24 * time.Hour, want: "7 days"},
		{name: "7 days 12 hours", duration: 7*24*time.Hour + 12*time.Hour, want: "7 days 12 hours"},
		// Edge cases: minutes/seconds hidden at larger scales
		{name: "1 hour hides seconds", duration: time.Hour + 30*time.Second, want: "1 hour"},
		{name: "1 day hides minutes", duration: 24*time.Hour + 30*time.Minute, want: "1 day"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := humanizeDuration(tt.duration); got != tt.want {
				t.Errorf("humanizeDuration() = %q, want %q", got, tt.want)
			}
		})
	}
}
