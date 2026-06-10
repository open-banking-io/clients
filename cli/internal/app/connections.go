package app

import (
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	openbanking "github.com/open-banking-io/clients/go"
)

func (a *App) connections(args []string) error {
	fs := flag.NewFlagSet("connections", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := a.client()
	if err != nil {
		return err
	}
	conns, err := client.GetConnections()
	if err != nil {
		return fmt.Errorf("could not list connections: %w", err)
	}
	return renderConnections(a.Stdout, conns)
}

func renderConnections(w io.Writer, conns []openbanking.Connection) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "BANK\tCOUNTRY\tSTATUS\tACCOUNTS\tPSU\tVALID UNTIL\tLAST SYNCED\tSESSION")
	for _, c := range conns {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			dash(c.AspspName), dash(c.AspspCountry), dash(c.Status), c.AccountCount,
			dash(c.PsuType), dash(c.ValidUntil), dash(c.LastSyncedAt), dash(c.SessionID))
	}
	return tw.Flush()
}
