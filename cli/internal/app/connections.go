package app

import (
	"flag"
	"fmt"
	"math"
	"time"

	"github.com/open-banking-io/clients/cli/internal/ui"
	openbanking "github.com/open-banking-io/clients/go"
)

func (a *App) connections(args []string) error {
	fs := flag.NewFlagSet("connections", flag.ContinueOnError)
	fs.SetOutput(a.stderr())
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := a.client()
	if err != nil {
		return err
	}
	stop := a.ui().Spinner("Fetching connections…")
	conns, err := client.GetConnections()
	stop()
	if err != nil {
		return fmt.Errorf("could not list connections: %w", err)
	}
	return a.ui().Render(connectionsTable(conns), connectionsView(conns))
}

func connectionsTable(conns []openbanking.Connection) ui.Table {
	t := ui.Table{Headers: []string{"BANK", "COUNTRY", "STATUS", "ACCESS", "ACCOUNTS", "PSU", "VALID UNTIL", "LAST SYNCED", "SESSION"}}
	for _, c := range conns {
		access := accessLabel(c)
		t.Rows = append(t.Rows, []ui.Cell{
			{Text: dash(c.AspspName)},
			{Text: dash(c.AspspCountry)},
			{Text: dash(c.Status), Style: ui.StatusStyle(c.Status)},
			{Text: access, Style: accessStyle(c)},
			{Text: fmt.Sprintf("%d", c.AccountCount)},
			{Text: dash(c.PsuType)},
			{Text: dash(c.ValidUntil)},
			{Text: dash(c.LastSyncedAt)},
			{Text: dash(c.SessionID), Style: ui.StyleMuted},
		})
	}
	return t
}

// accessLabel says whether the consent still works. Status stays "Active" after validUntil, so the
// service's isLive is the only honest answer; a live consent close to its end says how close.
func accessLabel(c openbanking.Connection) string {
	if !c.IsLive {
		return "ended"
	}
	if until, err := time.Parse(time.RFC3339, c.ValidUntil); err == nil {
		days := int(math.Ceil(time.Until(until).Hours() / 24))
		if days <= renewWithinDays {
			return fmt.Sprintf("live, %dd left", max(days, 1))
		}
	}
	return "live"
}

func accessStyle(c openbanking.Connection) ui.Style {
	switch label := accessLabel(c); {
	case !c.IsLive:
		return ui.StyleNegative
	case label != "live":
		return ui.StyleStatusWarn
	default:
		return ui.StyleStatusOK
	}
}

const renewWithinDays = 14

type connectionView struct {
	Bank         string   `json:"bank,omitempty"`
	Country      string   `json:"country,omitempty"`
	Status       string   `json:"status,omitempty"`
	AccountCount int64    `json:"accountCount"`
	PsuType      string   `json:"psuType,omitempty"`
	ValidUntil   string   `json:"validUntil,omitempty"`
	LastSyncedAt string   `json:"lastSyncedAt,omitempty"`
	SessionID    string   `json:"sessionId,omitempty"`
	IsLive       bool     `json:"isLive"`
	AccountIDs   []string `json:"accountIds"`
}

func connectionsView(conns []openbanking.Connection) []connectionView {
	views := make([]connectionView, 0, len(conns))
	for _, c := range conns {
		views = append(views, connectionView{
			Bank:         c.AspspName,
			Country:      c.AspspCountry,
			Status:       c.Status,
			AccountCount: c.AccountCount,
			PsuType:      c.PsuType,
			ValidUntil:   c.ValidUntil,
			LastSyncedAt: c.LastSyncedAt,
			SessionID:    c.SessionID,
			IsLive:       c.IsLive,
			AccountIDs:   c.AccountIDs,
		})
	}
	return views
}
