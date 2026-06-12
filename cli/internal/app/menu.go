package app

import (
	"fmt"

	"github.com/open-banking-io/clients/cli/internal/config"
	"github.com/open-banking-io/clients/cli/internal/ui"
)

// interactiveMenu is shown when openbanking is run with no command on a terminal: a short status
// header plus a selectable list of common actions. It is only reached interactively — non-terminal
// empty invocations print usage and error (see Run).
func (a *App) interactiveMenu() error {
	a.printStatus()

	actions := []ui.Option{
		{Label: "Accounts — list balances", Value: "accounts"},
		{Label: "Transactions — current account statement", Value: "transactions"},
		{Label: "Sync — pull fresh transactions", Value: "sync"},
		{Label: "Use — switch current account", Value: "use"},
		{Label: "Connections — bank connections", Value: "connections"},
		{Label: "Banks — available to connect", Value: "banks"},
		{Label: "Login — sign in", Value: "login"},
		{Label: "Quit", Value: ""},
	}
	choice, err := a.ui().Select("openbanking", actions)
	if err == ui.ErrCancelled {
		return nil
	}
	if err != nil {
		return err
	}
	if choice.Value == "" { // Quit
		return nil
	}
	c, ok := a.lookup(choice.Value)
	if !ok {
		return fmt.Errorf("unknown command %q", choice.Value)
	}
	return c.run(a, nil)
}

// printStatus writes a short who-am-I / current-account header to stderr.
func (a *App) printStatus() {
	bundle, err := config.Load(a.ConfigPath)
	if err != nil || bundle.APIKey == "" {
		a.ui().Infof("Not signed in — choose Login to get started.")
		return
	}
	user := bundle.User
	if user == "" {
		user = "signed in"
	}
	a.ui().Infof("Signed in as %s", user)
	if state, _ := config.LoadState(a.ConfigPath); state.CurrentAccountID != "" {
		a.ui().Infof("Current account: %s", state.CurrentAccountID)
	} else {
		a.ui().Infof("No current account set (choose Use to pick one)")
	}
}
