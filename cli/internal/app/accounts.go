package app

import (
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	openbanking "github.com/open-banking-io/clients/go"
)

func (a *App) accounts(args []string) error {
	fs := flag.NewFlagSet("accounts", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := a.client()
	if err != nil {
		return err
	}
	accounts, err := client.GetAccounts()
	if err != nil {
		return fmt.Errorf("could not list accounts: %w", err)
	}
	return renderAccounts(a.Stdout, accounts)
}

func renderAccounts(w io.Writer, accounts []openbanking.Account) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tIBAN\tTYPE\tBALANCE\tCUR\tBANK\tID")
	for _, acct := range accounts {
		bal := bookedBalance(acct.Balances)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			accountLabel(acct), accountNumber(acct), dash(acct.AccountType),
			dash(bal.Amount), dash(acct.Currency), dash(acct.AspspName), acct.ID)
	}
	return tw.Flush()
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
