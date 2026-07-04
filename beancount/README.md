# beancount-openbanking-io

A [Beancount](https://beancount.github.io/) / [beangulp](https://github.com/beancount/beangulp)
importer for [**open-banking.io**](https://open-banking.io) — zero-knowledge PSD2 bank
sync for EEA & UK banks.

Transactions arrive as zero-knowledge encrypted envelopes and are decrypted **locally** by
the official [`open-banking-io`](https://pypi.org/project/open-banking-io/) SDK with a key
only you hold. Your statement is never readable off your machine — a good fit for a
plaintext-accounting ledger you keep yourself.

## Why

If you sync EU/UK banks into Beancount, your options have been thinning out:

- **GoCardless / Nordigen** stopped approving new personal Bank-Account-Data accounts, and
  imposes a **4-requests-per-account-per-day** rate limit that breaks routine imports.
- Other aggregators terminate your bank connection **in the clear** on their servers.

`open-banking.io` is an alternative built for self-hosters: **client-side decryption**, **no
eIDAS certificate to obtain**, **no per-account daily rate-limit wall**.

## Install

```bash
pip install beancount-openbanking-io
```

This pulls in `beancount`, `beangulp`, and the `open-banking-io` SDK.

## Configure

Export your credentials bundle from the open-banking.io dashboard, then create a small YAML
file whose name ends in `openbanking.yaml`:

```yaml
# myfinances.openbanking.yaml
credentials: ./open-banking.io-credentials.json   # path is relative to this file
accounts:
  - id: f1345859-....          # open-banking.io account id (from client.get_accounts())
    account: Assets:Bank:Nordea:Checking
    currency: DKK              # optional guard
    counterpart: Expenses:Uncategorized   # optional; default holding account for the other leg
    assert_balance: true       # optional: emit a booked-balance (ITBD) assertion
```

To list your account ids:

```python
from open_banking_io import OpenBankingClient

with OpenBankingClient.from_credentials("open-banking.io-credentials.json") as client:
    for account in client.get_accounts():
        print(account.id, account.aspsp_name, account.currency, account.iban)
```

## Use

Wire it into your beangulp import script like any other importer:

```python
# import.py
import beangulp
from beancount_openbanking_io import OpenBankingImporter

if __name__ == "__main__":
    beangulp.Ingest([OpenBankingImporter()]).main()
```

```bash
python import.py extract myfinances.openbanking.yaml >> ledger.beancount
```

Re-running is safe: each entry is stamped with its stable open-banking.io transaction id
(`obio-id`), so already-imported transactions are skipped.

## Mapping

| open-banking.io | Beancount |
|---|---|
| `amount` (unsigned) + `creditDebitIndicator` | signed posting amount — **DBIT → negative, CRDT → positive** |
| `creditorName` (DBIT) / `debtorName` (CRDT) | `payee` |
| `remittanceInformation` / `note` | `narration` |
| `bookingDate` (→ `valueDate` → `transactionDate`) | entry `date` |
| `status` `PDNG` | `!` (flagged/uncleared); `BOOK` → `*` |
| transaction `id` | `obio-id` metadata (dedup key) |
| `ITBD` booked balance | optional `balance` assertion |

Each transaction gets a second, amount-elided posting to the `counterpart` account
(default `Expenses:Uncategorized`) so entries balance on import; reclassify it later
(e.g. with [`smart_importer`](https://github.com/beancount/smart_importer)). Transactions
whose direction cannot be determined are **skipped**, never imported with a guessed sign.

## License

MIT — see [LICENSE](LICENSE).
