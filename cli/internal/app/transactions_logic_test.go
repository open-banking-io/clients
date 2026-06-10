package app

import (
	"testing"

	openbanking "github.com/open-banking-io/clients/go"
)

func TestSignedAmount(t *testing.T) {
	cases := []struct {
		name      string
		indicator string
		amount    string
		want      string
	}{
		{"debit goes negative", "DBIT", "194.23", "-194.23"},
		{"credit stays positive", "CRDT", "194.23", "194.23"},
		{"debit lowercase still negative", "dbit", "10.00", "-10.00"},
		{"debit padded still negative", " DBIT ", "10.00", "-10.00"},
		{"debit zero stays zero", "DBIT", "0", "0"},
		{"empty amount becomes zero", "DBIT", "", "0"},
		{"already-negative debit not doubled", "DBIT", "-5.00", "-5.00"},
		{"unknown indicator stays positive", "", "7.50", "7.50"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := signedAmount(openbanking.Transaction{CreditDebitIndicator: c.indicator, Amount: c.amount})
			if got != c.want {
				t.Errorf("signedAmount(%q, %q) = %q, want %q", c.indicator, c.amount, got, c.want)
			}
		})
	}
}

func TestCounterparty(t *testing.T) {
	debit := openbanking.Transaction{CreditDebitIndicator: "DBIT", CreditorName: "One.com", DebtorName: "Me"}
	if got := counterparty(debit); got != "One.com" {
		t.Errorf("debit counterparty = %q, want creditor One.com", got)
	}
	credit := openbanking.Transaction{CreditDebitIndicator: "CRDT", CreditorName: "Me", DebtorName: "Acme"}
	if got := counterparty(credit); got != "Acme" {
		t.Errorf("credit counterparty = %q, want debtor Acme", got)
	}
	// Falls back across to the other party when the expected side is empty.
	fallback := openbanking.Transaction{CreditDebitIndicator: "DBIT", DebtorName: "OnlyDebtor"}
	if got := counterparty(fallback); got != "OnlyDebtor" {
		t.Errorf("debit fallback counterparty = %q, want OnlyDebtor", got)
	}
}
