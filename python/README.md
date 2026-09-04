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
            print(
                f"  {t.booking_date}  {t.creditor_name or t.debtor_name}  {t.amount} {t.currency}"
            )

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

## Diagnostics

When a call fails before any HTTP response, `diagnose()` probes each stage in turn and
returns a report that is safe to paste into a support ticket:

```python
with OpenBankingClient.from_credentials("credentials.json") as client:
    print(client.diagnose().report())
```

```
[PASS] base_url       'https://open-banking.io' -> host='open-banking.io' port=443 scheme=https  (0 ms)
[PASS] dns            open-banking.io -> 104.21.78.242, 172.67.138.186  (24 ms)
[PASS] tcp_connect    connected to open-banking.io:443  (10 ms)
[PASS] tls_handshake  TLSv1.3 TLS_AES_256_GCM_SHA384 issuer='Google Trust Services' notAfter=Nov  5 13:47:21 2026 GMT  (40 ms)
[PASS] api_preflight  GET api/accounts -> HTTP 200  (159 ms)
```

It never raises: a failing stage is recorded and the stages that depend on it are skipped, so
a DNS problem reads differently from a TLS problem or a rejected API key. The report carries
no API key (only a one-way fingerprint), no private key, no decrypted data and no response
bodies; proxy and CA environment variables are listed by name with their values withheld.
`as_dict()` returns the same data as JSON-serialisable structures.

Request logging is opt-in through the standard `logging` module and records only the method,
path, status and duration — never headers or bodies:

```python
import logging
logging.basicConfig()
logging.getLogger("open_banking_io").setLevel(logging.DEBUG)
```

## Encryption

Envelopes use **ECDH P-256 → HKDF-SHA256 → AES-256-GCM**. Decryption requires the private key from
your credentials bundle and happens entirely in-process. Full wire format and the other language
clients: [repo README](https://github.com/open-banking-io/clients) ·
[`THREAT_MODEL.md`](https://github.com/open-banking-io/clients/blob/main/THREAT_MODEL.md).

## Development

```bash
python -m venv .venv
.venv/bin/pip install -e .[dev]
.venv/bin/pytest -q
```

MIT licensed.
