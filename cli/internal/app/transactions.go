package app

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	openbanking "github.com/open-banking-io/clients/go"
)

func (a *App) transactions(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: obank transactions <account-id> [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--limit N] [--offset N]")
	}
	accountID := args[0]

	fs := flag.NewFlagSet("transactions", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	from := fs.String("from", "", "earliest booking date (YYYY-MM-DD)")
	to := fs.String("to", "", "latest booking date (YYYY-MM-DD)")
	limit := fs.Int("limit", 0, "max rows to return (0 = server default)")
	offset := fs.Int("offset", 0, "rows to skip")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	query := openbanking.TransactionQuery{From: *from, To: *to}
	if *limit > 0 {
		query.Limit = limit
	}
	if *offset > 0 {
		query.Offset = offset
	}

	client, err := a.client()
	if err != nil {
		return err
	}
	page, err := client.GetTransactions(accountID, query)
	if err != nil {
		return fmt.Errorf("could not list transactions: %w", err)
	}
	return renderTransactions(a.Stdout, page)
}

func renderTransactions(w io.Writer, page openbanking.TransactionPage) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "DATE\tAMOUNT\tCUR\tCOUNTERPARTY\tINFO\tSTATUS")
	for _, t := range page.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			dash(transactionDate(t)), signedAmount(t), dash(t.Currency),
			dash(counterparty(t)), dash(transactionInfo(t)), dash(t.Status))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "\n%d shown, %d total\n", len(page.Items), page.Total)
	return nil
}

// isDebit reports whether the indicator is a debit, tolerant of case and surrounding whitespace —
// a missed DBIT would silently flip the money direction, so the match must not be brittle.
func isDebit(indicator string) bool {
	return strings.EqualFold(strings.TrimSpace(indicator), "DBIT")
}

// signedAmount applies the sign at the presentation edge: amounts are stored unsigned with a
// credit/debit indicator, and a debit (money out) is shown negative.
func signedAmount(t openbanking.Transaction) string {
	amt := t.Amount
	if amt == "" {
		amt = "0"
	}
	if isDebit(t.CreditDebitIndicator) && amt != "0" && !strings.HasPrefix(amt, "-") {
		return "-" + amt
	}
	return amt
}

// counterparty is the other side of the transaction: for a debit the money goes to the creditor,
// for a credit it comes from the debtor.
func counterparty(t openbanking.Transaction) string {
	if isDebit(t.CreditDebitIndicator) {
		if t.CreditorName != "" {
			return t.CreditorName
		}
		return t.DebtorName
	}
	if t.DebtorName != "" {
		return t.DebtorName
	}
	return t.CreditorName
}

func transactionDate(t openbanking.Transaction) string {
	for _, d := range []string{t.BookingDate, t.ValueDate, t.TransactionDate} {
		if d != "" {
			return d
		}
	}
	return ""
}

func transactionInfo(t openbanking.Transaction) string {
	if t.RemittanceInformation != "" {
		return t.RemittanceInformation
	}
	return t.Note
}
