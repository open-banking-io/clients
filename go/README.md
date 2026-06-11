<p align="center">
  <a href="https://open-banking.io">
    <img src="https://raw.githubusercontent.com/open-banking-io/clients/main/.github/logo.png" alt="open-banking.io" height="56">
  </a>
</p>

# open-banking.io (Go)

Server-to-server client for [open-banking.io](https://open-banking.io): API-key auth +
client-side decryption of the zero-knowledge data envelopes with your exported private key.

```bash
go get github.com/open-banking-io/clients/go
```

```go
package main

import (
	"fmt"
	"log"

	openbanking "github.com/open-banking-io/clients/go"
)

func main() {
	client, err := openbanking.FromCredentials("credentials.json", nil)
	if err != nil {
		log.Fatal(err)
	}
	accounts, err := client.GetAccounts()
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range accounts {
		for _, b := range a.Balances {
			if b.Type == "ITBD" {
				fmt.Println(a.Iban, b.Amount, a.Currency)
			}
		}

		limit := 50
		page, err := client.GetTransactions(a.ID, openbanking.TransactionQuery{Limit: &limit})
		if err != nil {
			log.Fatal(err)
		}
		for _, t := range page.Items {
			fmt.Println(" ", t.BookingDate, t.CreditorName, t.Amount, t.Currency)
		}

		// Trigger an online sync (decrypts the account uid locally and posts it):
		if _, err := client.Sync(a.ID); err != nil {
			log.Fatal(err)
		}
	}
}
```

The package name is `openbanking`. It exposes the same surface as the other SDKs: `GetAccounts`,
`GetTransactions`, `GetConnections`, `Sync`, `SyncAll`. `Sync` decrypts the account's session uid
locally and posts it, so the service can refresh from the bank without ever holding it in plaintext.

Optional text fields are empty strings when absent; monetary amounts are returned as the string the
service emits — parse them into your decimal type of choice to avoid any float round-trip.

The client has **no third-party dependencies** — it uses only the standard library (`crypto/ecdh`,
`crypto/hkdf`, `crypto/aes`), which requires **Go 1.24+**.

When you pass a `nil` httpClient the client builds one with a 30s timeout; every request sends
`User-Agent: open-banking-io/go/<version>` (see the exported `Version` const).

## Encryption

Envelopes use **ECDH P-256 → HKDF-SHA256 → AES-256-GCM**, implemented with only the standard library
(`crypto/ecdh`, `crypto/hkdf`, `crypto/aes`). Decryption requires the private key from your
credentials bundle and happens entirely in-process. The package is verified against the shared
`fixtures/` so it decrypts identically to the other SDKs. Full wire format and the other language
clients: [repo README](https://github.com/open-banking-io/clients) ·
[`THREAT_MODEL.md`](https://github.com/open-banking-io/clients/blob/main/THREAT_MODEL.md).

## Development

```bash
go test ./...   # crypto round-trip + a mock-server integration suite
```

MIT licensed.
