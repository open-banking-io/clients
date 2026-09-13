package app

import (
	"errors"
	"flag"
	"fmt"

	"github.com/open-banking-io/clients/cli/internal/ui"
	openbanking "github.com/open-banking-io/clients/go"
)

func (a *App) sync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(a.stderr())
	all := fs.Bool("all", false, "sync every account that has an active session")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := a.client()
	if err != nil {
		return err
	}

	if *all {
		if fs.NArg() > 0 {
			return fmt.Errorf("`sync --all` takes no account id")
		}
		stop := a.ui().Spinner("Syncing all accounts…")
		result, err := client.SyncAll()
		stop()
		if err != nil {
			return fmt.Errorf("sync failed: %w", err)
		}
		fmt.Fprintln(a.stdout(), a.ui().Color(
			fmt.Sprintf("Synced %d account(s): %d new transaction(s)", result.Accounts, result.NewTransactions),
			ui.StyleSuccess))
		for _, f := range result.Failures {
			line := fmt.Sprintf("  %s  %s", f.AccountID, f.Reason)
			if f.BankErrorCode != "" {
				line += " (" + f.BankErrorCode + ")"
			}
			if hint := syncHint(f.Reason); hint != "" {
				line += " — " + hint
			}
			fmt.Fprintln(a.stderr(), a.ui().Color(line, ui.StyleStatusWarn))
		}
		if n := len(result.Failures); n > 0 {
			return fmt.Errorf("%d account(s) could not be synced", n)
		}
		return nil
	}

	// No id and no --all: fall back to the current account (or the picker on a terminal), rather
	// than erroring outright.
	accountID, err := a.resolveAccountID(fs.Arg(0))
	if err != nil {
		return err
	}
	stop := a.ui().Spinner("Syncing…")
	result, err := client.Sync(accountID)
	stop()
	if err != nil {
		var refused *openbanking.SyncError
		if errors.As(err, &refused) {
			if hint := syncHint(refused.Reason); hint != "" {
				return fmt.Errorf("sync failed: %w — %s", err, hint)
			}
		}
		return fmt.Errorf("sync failed: %w", err)
	}
	fmt.Fprintln(a.stdout(), a.ui().Color(
		fmt.Sprintf("Synced: %d new transaction(s) (%d fetched)", result.NewTransactions, result.TotalFetched),
		ui.StyleSuccess))
	return nil
}

// syncHint is what to do about a refusal, keyed on the stable reason the service sends.
func syncHint(reason string) string {
	switch reason {
	case openbanking.ReasonReconnectNeeded:
		return "reconnect this bank at open-banking.io"
	case openbanking.ReasonPsuPresentRequired:
		return "this bank only shares data while you use the web app; sync there"
	case openbanking.ReasonRateLimited:
		return "the bank is throttling; try again later"
	case openbanking.ReasonConsentWithdrawn:
		return "this account was removed from its connection"
	default:
		return ""
	}
}
