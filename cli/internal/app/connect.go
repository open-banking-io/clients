package app

import (
	"flag"
	"fmt"
	"strings"
	"time"

	openbanking "github.com/open-banking-io/clients/go"
)

// connectPollInterval is how often `connect` polls for the new connection while the user completes
// consent in the browser. A package var so tests can shorten it.
var connectPollInterval = 2 * time.Second

func (a *App) connect(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: openbanking connect <bank-name> [--country DK] [--psu-type personal|business]")
	}
	bankName := args[0]

	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	country := fs.String("country", "DK", "bank country")
	psuType := fs.String("psu-type", "", "account type to grant: personal or business")
	timeout := fs.Duration("timeout", 3*time.Minute, "how long to wait for consent")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	client, err := a.publicClient()
	if err != nil {
		return err
	}

	// Snapshot existing connections so we can tell which one the consent flow adds.
	before, err := client.GetConnections()
	if err != nil {
		return fmt.Errorf("could not read existing connections: %w", err)
	}
	seen := make(map[string]bool, len(before))
	for _, c := range before {
		seen[c.SessionID] = true
	}

	consentURL, err := client.StartAuthorization(openbanking.AuthorizationRequest{
		Country: *country, AspspName: bankName, PsuType: *psuType,
	})
	if err != nil {
		return fmt.Errorf("could not start authorization: %w", err)
	}

	fmt.Fprintf(a.Stderr, "Opening your browser to authorize %s...\nIf it doesn't open, visit:\n  %s\n\n", bankName, consentURL)
	if err := openBrowser(consentURL); err != nil {
		fmt.Fprintf(a.Stderr, "(could not open a browser automatically: %v)\n", err)
	}

	// The bank's callback completes the consent server-side and runs the first sync, after which a
	// new connection appears. Poll until it does (or we time out).
	deadline := time.Now().Add(*timeout)
	for {
		time.Sleep(connectPollInterval)
		if conns, err := client.GetConnections(); err == nil {
			for _, c := range conns {
				if !seen[c.SessionID] {
					fmt.Fprintf(a.Stdout, "Connected %s (%s) — %d account(s).\n",
						c.AspspName, c.AspspCountry, c.AccountCount)
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for the bank connection after %s", *timeout)
		}
	}
}
