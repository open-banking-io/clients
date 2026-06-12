package app

import (
	"flag"
	"fmt"

	"github.com/open-banking-io/clients/cli/internal/ui"
	openbanking "github.com/open-banking-io/clients/go"
)

func (a *App) accounts(args []string) error {
	fs := flag.NewFlagSet("accounts", flag.ContinueOnError)
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
	return a.ui().Render(accountsTable(accounts), accountsView(accounts))
}

func accountsTable(accounts []openbanking.Account) ui.Table {
	t := ui.Table{Headers: []string{"NAME", "IBAN", "TYPE", "BALANCE", "CUR", "BANK", "ID"}}
	for _, acct := range accounts {
		bal := bookedBalance(acct.Balances)
		balCell := ui.Cell{Text: dash(bal.Amount)}
		if bal.Amount != "" {
			balCell.Style = ui.AmountStyle(bal.Amount)
		}
		t.Rows = append(t.Rows, []ui.Cell{
			{Text: accountLabel(acct)},
			{Text: accountNumber(acct)},
			{Text: dash(acct.AccountType)},
			balCell,
			{Text: dash(acct.Currency)},
			{Text: dash(acct.AspspName)},
			{Text: acct.ID, Style: ui.StyleMuted},
		})
	}
	return t
}

// accountView is the stable, CLI-shaped JSON representation of an account (lowerCamel keys, only the
// fields the CLI surfaces) — deliberately decoupled from the untagged SDK struct.
type accountView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Number   string `json:"accountNumber,omitempty"`
	Type     string `json:"type,omitempty"`
	Balance  string `json:"balance,omitempty"`
	Currency string `json:"currency,omitempty"`
	Bank     string `json:"bank,omitempty"`
}

func accountsView(accounts []openbanking.Account) []accountView {
	views := make([]accountView, 0, len(accounts))
	for _, acct := range accounts {
		number := acct.Iban
		if number == "" {
			number = acct.Bban
		}
		views = append(views, accountView{
			ID:       acct.ID,
			Name:     accountLabel(acct),
			Number:   number,
			Type:     acct.AccountType,
			Balance:  bookedBalance(acct.Balances).Amount,
			Currency: acct.Currency,
			Bank:     acct.AspspName,
		})
	}
	return views
}

// accountLabel is the most human-friendly name available for an account.
func accountLabel(a openbanking.Account) string {
	for _, s := range []string{a.DisplayName, a.AccountName, a.OwnerName, a.Product} {
		if s != "" {
			return s
		}
	}
	return a.ID
}

// accountNumber prefers IBAN, falling back to BBAN.
func accountNumber(a openbanking.Account) string {
	if a.Iban != "" {
		return a.Iban
	}
	return dash(a.Bban)
}

// bookedBalance returns the booked balance (ISO 20022 ITBD), falling back to interim-available
// (ITAV) and then the first balance present.
func bookedBalance(balances []openbanking.Balance) openbanking.Balance {
	for _, b := range balances {
		if b.Type == "ITBD" {
			return b
		}
	}
	for _, b := range balances {
		if b.Type == "ITAV" {
			return b
		}
	}
	if len(balances) > 0 {
		return balances[0]
	}
	return openbanking.Balance{}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
