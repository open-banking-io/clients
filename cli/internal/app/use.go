package app

import (
	"flag"
	"fmt"

	"github.com/open-banking-io/clients/cli/internal/config"
)

// use sets the current account that transactions and sync default to. With an account id it sets
// it directly (after checking it exists); with no id it opens the interactive picker on a terminal.
func (a *App) use(args []string) error {
	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	fs.SetOutput(a.stderr())
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := a.client()
	if err != nil {
		return err
	}
	stop := a.ui().Spinner("Fetching accounts…")
	accounts, err := client.GetAccounts()
	stop()
	if err != nil {
		return fmt.Errorf("could not list accounts: %w", err)
	}
	if len(accounts) == 0 {
		return fmt.Errorf("no accounts found — connect a bank in the web app, then run `openbanking sync`")
	}

	var chosenID string
	if id := fs.Arg(0); id != "" {
		if !accountExists(accounts, id) {
			return fmt.Errorf("unknown account id %q (run `openbanking accounts` to list them)", id)
		}
		chosenID = id
	} else {
		if !a.ui().Interactive() {
			return fmt.Errorf("no account id given and not a terminal — pass one, e.g. `openbanking use <account-id>`")
		}
		if chosenID, err = a.pickAccount(accounts); err != nil {
			return err
		}
	}

	if err := config.SaveState(a.ConfigPath, config.State{CurrentAccountID: chosenID}); err != nil {
		return err
	}
	for _, acct := range accounts {
		if acct.ID == chosenID {
			a.ui().Successf("Current account set to %s (%s)", accountLabel(acct), acct.ID)
			return nil
		}
	}
	a.ui().Successf("Current account set to %s", chosenID)
	return nil
}
