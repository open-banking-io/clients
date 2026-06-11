<p align="center">
  <a href="https://open-banking.io">
    <img src="https://raw.githubusercontent.com/open-banking-io/clients/main/.github/logo.png" alt="open-banking.io" height="56">
  </a>
</p>

# open-banking-io (Rust)

Server-to-server client for [open-banking.io](https://open-banking.io): API-key auth +
client-side decryption of the zero-knowledge data envelopes with your exported private key.

```toml
[dependencies]
open-banking-io = "0.2"
```

```rust
use open_banking_io::{OpenBankingClient, TransactionQuery};

fn main() -> open_banking_io::Result<()> {
    // Load the credentials .json you exported from the app (API key + private key).
    let client = OpenBankingClient::from_credentials("credentials.json")?;

    for a in client.get_accounts()? {
        let booked = a.balances.iter().find(|b| b.type_ == "ITBD");
        println!("{:?}: {:?} {}", a.iban, booked.map(|b| &b.amount), a.currency);

        let page = client.get_transactions(&a.id, &TransactionQuery { limit: Some(50), ..Default::default() })?;
        for t in &page.items {
            println!("  {:?}  {:?}  {} {}", t.booking_date, t.creditor_name, t.amount, t.currency);
        }

        // Trigger an online sync (decrypts the account uid locally and posts it):
        client.sync(&a.id)?;
    }
    Ok(())
}
```

The client is synchronous (built on [`ureq`](https://crates.io/crates/ureq)). It exposes the same
surface as the other SDKs: `get_accounts`, `get_transactions`, `get_connections`, `sync`,
`sync_all`. `sync` decrypts the account's session uid locally and posts it, so the service can
refresh from the bank without ever holding it in plaintext.

Requests carry sensible HTTP timeouts (10s connect / 30s overall) and a
`User-Agent: open-banking-io/rust/<version>`.

Monetary amounts are returned as `String` exactly as the service emits them — parse them into your
decimal type of choice to avoid any float round-trip.

## Encryption

Envelopes use **ECDH P-256 → HKDF-SHA256 → AES-256-GCM**; decryption requires the private key from
your credentials bundle and happens entirely in-process (the derived key material is zeroized after
use). The crate is verified against the shared `fixtures/` so it decrypts identically to the other
SDKs. Full wire format and the other language clients:
[repo README](https://github.com/open-banking-io/clients) ·
[`THREAT_MODEL.md`](https://github.com/open-banking-io/clients/blob/main/THREAT_MODEL.md).

## Development

```bash
cargo test    # crypto round-trip + a mock-server integration suite
```

MIT licensed.
