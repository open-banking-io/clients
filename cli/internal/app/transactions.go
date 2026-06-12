package app

import (
	"flag"
	"fmt"
	"strings"

	"github.com/open-banking-io/clients/cli/internal/ui"
	openbanking "github.com/open-banking-io/clients/go"
)

func (a *App) transactions(args []string) error {
	// A leading non-flag token is an explicit account id; otherwise the current account is used (or
	// the picker opens). Either way the remaining tokens are the command's own flags.
	explicit := ""
	flagArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		explicit = args[0]
		flagArgs = args[1:]
	}

	fs := flag.NewFlagSet("transactions", flag.ContinueOnError)
	fs.SetOutput(a.stderr())
	from := fs.String("from", "", "earliest booking date (YYYY-MM-DD)")
	to := fs.String("to", "", "latest booking date (YYYY-MM-DD)")
	limit := fs.Int("limit", 0, "max rows to return (0 = server default)")
	offset := fs.Int("offset", 0, "rows to skip")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	accountID, err := a.resolveAccountID(explicit)
	if err != nil {
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
	stop := a.ui().Spinner("Fetching transactions…")
	page, err := client.GetTransactions(accountID, query)
	stop()
	if err != nil {
		return fmt.Errorf("could not list transactions: %w", err)
	}
	return a.ui().Render(transactionsTable(page), transactionsView(page))
}

func transactionsTable(page openbanking.TransactionPage) ui.Table {
	t := ui.Table{Headers: []string{"DATE", "AMOUNT", "CUR", "COUNTERPARTY", "INFO", "STATUS"}}
	for _, tx := range page.Items {
		amt := signedAmount(tx)
		t.Rows = append(t.Rows, []ui.Cell{
			{Text: dash(transactionDate(tx))},
			{Text: amt, Style: ui.AmountStyle(amt)},
			{Text: dash(tx.Currency)},
			{Text: dash(counterparty(tx))},
			{Text: dash(transactionInfo(tx))},
			{Text: dash(tx.Status), Style: ui.StatusStyle(tx.Status)},
		})
	}
	t.Note = fmt.Sprintf("%d shown, %d total", len(page.Items), page.Total)
	return t
}

type transactionView struct {
	ID           string `json:"id"`
	Date         string `json:"date,omitempty"`
	Amount       string `json:"amount"`
	Currency     string `json:"currency,omitempty"`
	Counterparty string `json:"counterparty,omitempty"`
	Info         string `json:"info,omitempty"`
	Status       string `json:"status,omitempty"`
}

type transactionsPageView struct {
	Items []transactionView `json:"items"`
	Shown int               `json:"shown"`
	Total int64             `json:"total"`
}

func transactionsView(page openbanking.TransactionPage) transactionsPageView {
	view := transactionsPageView{
		Items: make([]transactionView, 0, len(page.Items)),
		Shown: len(page.Items),
		Total: page.Total,
	}
	for _, tx := range page.Items {
		view.Items = append(view.Items, transactionView{
			ID:           tx.ID,
			Date:         transactionDate(tx),
			Amount:       signedAmount(tx),
			Currency:     tx.Currency,
			Counterparty: counterparty(tx),
			Info:         transactionInfo(tx),
			Status:       tx.Status,
		})
	}
	return view
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
