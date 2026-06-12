package app

import (
	"fmt"

	"github.com/open-banking-io/clients/cli/internal/config"
	"github.com/open-banking-io/clients/cli/internal/ui"
	openbanking "github.com/open-banking-io/clients/go"
)

// resolveAccountID decides which account a command should act on when the user didn't name one on
// the command line. An explicit, non-empty id always wins. Otherwise the saved current account is
// used; if none is set, the picker opens on a terminal, or a clear error is returned in a
// non-interactive context (so scripts fail loudly instead of hanging on a prompt).
//
// The saved id is trusted without a validating fetch — the common path stays a single API call. If
// the saved account has since been removed, the command's own request returns a clear error and the
// user can re-pick with `openbanking use`.
func (a *App) resolveAccountID(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	if state, _ := config.LoadState(a.ConfigPath); state.CurrentAccountID != "" {
		return state.CurrentAccountID, nil
	}

	if !a.ui().Interactive() {
		return "", fmt.Errorf("no account given and no current account set — pass an account id or run `openbanking use`")
	}

	client, err := a.client()
	if err != nil {
		return "", err
	}
	stop := a.ui().Spinner("Fetching accounts…")
	accounts, err := client.GetAccounts()
	stop()
	if err != nil {
		return "", fmt.Errorf("could not list accounts: %w", err)
	}
	if len(accounts) == 0 {
		return "", fmt.Errorf("no accounts found — connect a bank in the web app, then run `openbanking sync`")
	}

	id, err := a.pickAccount(accounts)
	if err != nil {
		return "", err
	}
	// Remember the choice so the next call needs no prompt.
	_ = config.SaveState(a.ConfigPath, config.State{CurrentAccountID: id})
	return id, nil
}

func accountExists(accounts []openbanking.Account, id string) bool {
	for _, acct := range accounts {
		if acct.ID == id {
			return true
		}
	}
	return false
}

// pickAccount opens the interactive selector over accounts and returns the chosen account id.
func (a *App) pickAccount(accounts []openbanking.Account) (string, error) {
	opts := make([]ui.Option, len(accounts))
	for i, acct := range accounts {
		opts[i] = ui.Option{
			Label: fmt.Sprintf("%s  %s  %s", accountLabel(acct), accountNumber(acct), dash(acct.AspspName)),
			Value: acct.ID,
		}
	}
	choice, err := a.ui().Select("Select an account", opts)
	if err != nil {
		return "", err
	}
	return choice.Value, nil
}
