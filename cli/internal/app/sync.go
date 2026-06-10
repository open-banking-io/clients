package app

import (
	"flag"
	"fmt"
)

func (a *App) sync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
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
		result, err := client.SyncAll()
		if err != nil {
			return fmt.Errorf("sync failed: %w", err)
		}
		fmt.Fprintf(a.Stdout, "Synced %d account(s): %d new transaction(s)\n",
			result.Accounts, result.NewTransactions)
		return nil
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("usage: openbanking sync <account-id>   (or: openbanking sync --all)")
	}
	result, err := client.Sync(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}
	fmt.Fprintf(a.Stdout, "Synced: %d new transaction(s) (%d fetched)\n",
		result.NewTransactions, result.TotalFetched)
	return nil
}
