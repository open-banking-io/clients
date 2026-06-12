package app

import (
	"flag"
	"fmt"

	"github.com/open-banking-io/clients/cli/internal/ui"
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
		return fmt.Errorf("sync failed: %w", err)
	}
	fmt.Fprintln(a.stdout(), a.ui().Color(
		fmt.Sprintf("Synced: %d new transaction(s) (%d fetched)", result.NewTransactions, result.TotalFetched),
		ui.StyleSuccess))
	return nil
}
