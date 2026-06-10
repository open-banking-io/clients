package app

import (
	"testing"

	openbanking "github.com/open-banking-io/clients/go"
)

func TestBookedBalance(t *testing.T) {
	itbd := openbanking.Balance{Type: "ITBD", Amount: "100"}
	itav := openbanking.Balance{Type: "ITAV", Amount: "90"}
	othr := openbanking.Balance{Type: "OTHR", Amount: "5"}

	cases := []struct {
		name     string
		balances []openbanking.Balance
		want     string // expected Amount
	}{
		{"prefers ITBD", []openbanking.Balance{othr, itav, itbd}, "100"},
		{"falls back to ITAV", []openbanking.Balance{othr, itav}, "90"},
		{"falls back to first", []openbanking.Balance{othr}, "5"},
		{"empty yields zero balance", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bookedBalance(c.balances).Amount; got != c.want {
				t.Errorf("bookedBalance amount = %q, want %q", got, c.want)
			}
		})
	}
}

func TestAccountNumber(t *testing.T) {
	if got := accountNumber(openbanking.Account{Iban: "DK123", Bban: "999"}); got != "DK123" {
		t.Errorf("with IBAN = %q, want DK123", got)
	}
	if got := accountNumber(openbanking.Account{Bban: "999"}); got != "999" {
		t.Errorf("without IBAN = %q, want BBAN 999", got)
	}
	if got := accountNumber(openbanking.Account{}); got != "-" {
		t.Errorf("with neither = %q, want dash", got)
	}
}

func TestAccountLabel(t *testing.T) {
	cases := []struct {
		name string
		acct openbanking.Account
		want string
	}{
		{"display name wins", openbanking.Account{DisplayName: "Drift", AccountName: "x", ID: "id"}, "Drift"},
		{"account name next", openbanking.Account{AccountName: "Acct", OwnerName: "x", ID: "id"}, "Acct"},
		{"owner name next", openbanking.Account{OwnerName: "Tatic", Product: "x", ID: "id"}, "Tatic"},
		{"product next", openbanking.Account{Product: "Current", ID: "id"}, "Current"},
		{"id is last resort", openbanking.Account{ID: "acc-1"}, "acc-1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := accountLabel(c.acct); got != c.want {
				t.Errorf("accountLabel = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDash(t *testing.T) {
	if got := dash(""); got != "-" {
		t.Errorf("dash(\"\") = %q, want -", got)
	}
	if got := dash("x"); got != "x" {
		t.Errorf("dash(\"x\") = %q, want x", got)
	}
}

func TestTransactionDate(t *testing.T) {
	cases := []struct {
		name string
		tx   openbanking.Transaction
		want string
	}{
		{"prefers booking date", openbanking.Transaction{BookingDate: "2026-01-01", ValueDate: "2026-01-02"}, "2026-01-01"},
		{"falls back to value date", openbanking.Transaction{ValueDate: "2026-01-02", TransactionDate: "2026-01-03"}, "2026-01-02"},
		{"falls back to transaction date", openbanking.Transaction{TransactionDate: "2026-01-03"}, "2026-01-03"},
		{"empty when none", openbanking.Transaction{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := transactionDate(c.tx); got != c.want {
				t.Errorf("transactionDate = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTransactionInfo(t *testing.T) {
	if got := transactionInfo(openbanking.Transaction{RemittanceInformation: "Invoice 7", Note: "n"}); got != "Invoice 7" {
		t.Errorf("with remittance = %q, want Invoice 7", got)
	}
	if got := transactionInfo(openbanking.Transaction{Note: "memo"}); got != "memo" {
		t.Errorf("falls back to note = %q, want memo", got)
	}
	if got := transactionInfo(openbanking.Transaction{}); got != "" {
		t.Errorf("empty = %q, want empty", got)
	}
}
