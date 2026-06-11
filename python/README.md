<p align="center">
  <a href="https://open-banking.io">
    <img src="https://raw.githubusercontent.com/open-banking-io/clients/main/.github/logo.png" alt="open-banking.io" height="56">
  </a>
</p>

# open-banking-io (Python)

Server-to-server client for [open-banking.io](https://open-banking.io). It authenticates with your
**API key** and decrypts the **zero-knowledge** data envelopes locally with your exported **private
key** — the service only ever returns ciphertext it cannot read.

```bash
pip install open-banking-io
```

```python
from open_banking_io import OpenBankingClient

# Load the credentials .json you exported from the app (API key + private key).
with OpenBankingClient.from_credentials("credentials.json") as client:
    for account in client.get_accounts():
        booked = next((b for b in account.balances if b.type == "ITBD"), None)
        label = account.display_name or account.owner_name
        print(f"{label} {account.iban}: {booked.amount if booked else None} {account.currency}")

        page = client.get_transactions(account.id, limit=50)
        for t in page.items:
            print(f"  {t.booking_date}  {t.creditor_name or t.debtor_name}  {t.amount} {t.currency}")

    # Trigger an online sync (decrypts the account uid locally and posts it):
    client.sync(account.id)
```

Or construct it explicitly:

```python
client = OpenBankingClient(api_base_url, api_key, private_key_pkcs8)
```

## API

- `get_accounts() -> list[Account]` — decrypts each account's envelope, display name and balances.
- `get_transactions(account_id, *, date_from=None, date_to=None, limit=None, offset=None) -> TransactionPage`
- `get_connections() -> list[Connection]`
- `sync(account_id) -> SyncResult` — decrypts the account uid locally and posts it.
- `sync_all() -> SyncAllResult` — syncs every account that has an active session.

Amounts are exposed as `decimal.Decimal`. Models are plain `@dataclass`es.

When the client constructs its own `httpx.Client` it applies a default 30s timeout and sends a `User-Agent: open-banking-io/python/<version>` header on every request (a caller-supplied client is left untouched).

## Encryption

Envelopes use **ECDH P-256 → HKDF-SHA256 → AES-256-GCM**. Decryption requires the private key from
your credentials bundle and happens entirely in-process. See the
[repo README](https://github.com/open-banking-io/clients) for the full scheme and the other language
clients (.NET, Node).

## Development

```bash
python -m venv .venv
.venv/bin/pip install -e .[dev]
.venv/bin/pytest -q
```

MIT licensed.
