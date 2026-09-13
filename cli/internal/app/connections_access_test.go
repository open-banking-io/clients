package app

import (
	"testing"
	"time"

	openbanking "github.com/open-banking-io/clients/go"
)

func TestAccessLabel(t *testing.T) {
	in := func(d time.Duration) string { return time.Now().Add(d).UTC().Format(time.RFC3339) }
	cases := []struct {
		name string
		conn openbanking.Connection
		want string
	}{
		{"ended even with time left", openbanking.Connection{IsLive: false, ValidUntil: in(90 * 24 * time.Hour)}, "ended"},
		{"far from its end", openbanking.Connection{IsLive: true, ValidUntil: in(15*24*time.Hour + time.Hour)}, "live"},
		{"inside the renewal window", openbanking.Connection{IsLive: true, ValidUntil: in(6*24*time.Hour + time.Hour)}, "live, 7d left"},
		{"the last hours are one day", openbanking.Connection{IsLive: true, ValidUntil: in(3 * time.Hour)}, "live, 1d left"},
		{"the service's clock wins", openbanking.Connection{IsLive: true, ValidUntil: in(-time.Hour)}, "live, 1d left"},
	}
	for _, c := range cases {
		if got := accessLabel(c.conn); got != c.want {
			t.Errorf("%s: accessLabel = %q, want %q", c.name, got, c.want)
		}
	}
}
