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

## Encryption scheme

Each sensitive value is an envelope: `version(1) | ephemeralPublicKey(65) | nonce(12) | tag(16) |
ciphertext`, produced with ephemeral ECDH on P-256 → HKDF-SHA256 (salt = 32 zero bytes, info =
`bank.core.ci/zk/v1`) → AES-256-GCM. Only your private key can open it. The package is verified
against the shared `fixtures/` so it decrypts identically to the other SDKs.

## Development

```bash
go test ./...   # crypto round-trip + a mock-server integration suite
```

MIT licensed.
