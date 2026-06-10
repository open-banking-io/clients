# open-banking-io (Rust)

Server-to-server client for [open-banking.io](https://open-banking.io): API-key auth +
client-side decryption of the zero-knowledge data envelopes with your exported private key.

```toml
[dependencies]
open-banking-io = "0.1"
```

```rust
use open_banking_io::OpenBankingClient;

fn main() -> open_banking_io::Result<()> {
    let client = OpenBankingClient::from_credentials("credentials.json")?;
    for a in client.get_accounts()? {
        let booked = a.balances.iter().find(|b| b.type_ == "ITBD");
        println!("{:?}: {:?} {}", a.iban, booked.map(|b| &b.amount), a.currency);
    }
    Ok(())
}
```

The client is synchronous (built on [`ureq`](https://crates.io/crates/ureq)). It exposes the same
surface as the other SDKs: `get_accounts`, `get_transactions`, `get_connections`, `sync`,
`sync_all`. `sync` decrypts the account's session uid locally and posts it, so the service can
refresh from the bank without ever holding it in plaintext.

Monetary amounts are returned as `String` exactly as the service emits them — parse them into your
decimal type of choice to avoid any float round-trip.

## Encryption scheme

Each sensitive value is an envelope: `version(1) | ephemeralPublicKey(65) | nonce(12) | tag(16) |
ciphertext`, produced with ephemeral ECDH on P-256 → HKDF-SHA256 (salt = 32 zero bytes, info =
`bank.core.ci/zk/v1`) → AES-256-GCM. Only your private key can open it. The crate is verified
against the shared `fixtures/` so it decrypts identically to the .NET, Node, Python, Go, and Java
SDKs.

## Development

```bash
cargo test    # crypto round-trip + a mock-server integration suite
```

MIT licensed.
