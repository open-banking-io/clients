package app

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	openbanking "github.com/open-banking-io/clients/go"
)

func (a *App) banks(args []string) error {
	fs := flag.NewFlagSet("banks", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	country := fs.String("country", "DK", "country to list banks for")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := a.publicClient()
	if err != nil {
		return err
	}
	banks, err := client.ListBanks(*country)
	if err != nil {
		return fmt.Errorf("could not list banks: %w", err)
	}
	return renderBanks(a.Stdout, banks)
}

func renderBanks(w io.Writer, banks []openbanking.Bank) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tCOUNTRY\tBIC\tPSU TYPES\tBETA")
	for _, b := range banks {
		beta := ""
		if b.Beta {
			beta = "beta"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			dash(b.Name), dash(b.Country), dash(b.Bic), dash(strings.Join(b.PsuTypes, ",")), beta)
	}
	return tw.Flush()
}
