package main

import (
	"testing"
	"time"
)

// A close shows the keyboard again; a second close within 2 s hides it, so
// a compositor with no output for it cannot make the app loop.
func TestReattachAfterClose(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name string
		last time.Time
		want bool
	}{
		{"first close", time.Time{}, true},
		{"again after 1 s", now.Add(-time.Second), false},
		{"again just under 2 s", now.Add(-1999 * time.Millisecond), false},
		{"again after 2 s", now.Add(-2 * time.Second), true},
		{"again after a minute", now.Add(-time.Minute), true},
	} {
		if got := reattachAfterClose(now, tt.last); got != tt.want {
			t.Errorf("%s: reattach %v, want %v", tt.name, got, tt.want)
		}
	}
}
