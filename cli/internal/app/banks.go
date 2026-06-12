package app

import (
	"flag"
	"fmt"
	"strings"

	"github.com/open-banking-io/clients/cli/internal/ui"
	openbanking "github.com/open-banking-io/clients/go"
)

func (a *App) banks(args []string) error {
	fs := flag.NewFlagSet("banks", flag.ContinueOnError)
	fs.SetOutput(a.stderr())
	country := fs.String("country", "DK", "country to list banks for")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := a.publicClient()
	if err != nil {
		return err
	}
	stop := a.ui().Spinner("Fetching banks…")
	banks, err := client.ListBanks(*country)
	stop()
	if err != nil {
		return fmt.Errorf("could not list banks: %w", err)
	}
	return a.ui().Render(banksTable(banks), banksView(banks))
}

func banksTable(banks []openbanking.Bank) ui.Table {
	t := ui.Table{Headers: []string{"NAME", "COUNTRY", "BIC", "PSU TYPES", "BETA"}}
	for _, b := range banks {
		beta := ui.Cell{}
		if b.Beta {
			beta = ui.Cell{Text: "beta", Style: ui.StyleStatusWarn}
		}
		t.Rows = append(t.Rows, []ui.Cell{
			{Text: dash(b.Name)},
			{Text: dash(b.Country)},
			{Text: dash(b.Bic)},
			{Text: dash(strings.Join(b.PsuTypes, ","))},
			beta,
		})
	}
	return t
}

type bankView struct {
	Name     string   `json:"name"`
	Country  string   `json:"country,omitempty"`
	Bic      string   `json:"bic,omitempty"`
	PsuTypes []string `json:"psuTypes,omitempty"`
	Beta     bool     `json:"beta"`
}

func banksView(banks []openbanking.Bank) []bankView {
	views := make([]bankView, 0, len(banks))
	for _, b := range banks {
		views = append(views, bankView{
			Name:     b.Name,
			Country:  b.Country,
			Bic:      b.Bic,
			PsuTypes: b.PsuTypes,
			Beta:     b.Beta,
		})
	}
	return views
}
