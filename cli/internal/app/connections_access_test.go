package app

import (
	"testing"
	"time"

	"github.com/open-banking-io/clients/cli/internal/ui"
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
		{"just past the window", openbanking.Connection{IsLive: true, ValidUntil: in(14*24*time.Hour + time.Hour)}, "live"},
		{"just inside the window", openbanking.Connection{IsLive: true, ValidUntil: in(14*24*time.Hour - time.Hour)}, "live, 14d left"},
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

func TestAccessStyle(t *testing.T) {
	in := func(d time.Duration) string { return time.Now().Add(d).UTC().Format(time.RFC3339) }
	if got := accessStyle(openbanking.Connection{IsLive: false}); got != ui.StyleNegative {
		t.Errorf("ended style = %v", got)
	}
	if got := accessStyle(openbanking.Connection{IsLive: true, ValidUntil: in(3 * 24 * time.Hour)}); got != ui.StyleStatusWarn {
		t.Errorf("ending style = %v", got)
	}
	if got := accessStyle(openbanking.Connection{IsLive: true, ValidUntil: in(90 * 24 * time.Hour)}); got != ui.StyleStatusOK {
		t.Errorf("live style = %v", got)
	}
}

func TestConnectionsView_CarriesLivenessAndAccounts(t *testing.T) {
	views := connectionsView([]openbanking.Connection{{SessionID: "s1", IsLive: true, AccountIDs: []string{"a1", "a2"}}})

	if !views[0].IsLive || len(views[0].AccountIDs) != 2 || views[0].AccountIDs[1] != "a2" {
		t.Errorf("view = %+v", views[0])
	}
}
