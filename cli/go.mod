module github.com/open-banking-io/clients/cli

go 1.26.0

require (
	github.com/open-banking-io/clients/go/v2 v2.0.0
	golang.org/x/term v0.48.0
)

// The SDK's v2 module path resolves via the workspace in CI; this replace keeps
// standalone (GOWORK=off) builds — e.g. the publish-cli tag builds — working
// before and after go/v2.0.0 reaches the module proxy. Remove after v2 is proxied.
replace github.com/open-banking-io/clients/go/v2 => ../go

require golang.org/x/sys v0.50.0 // indirect
