<div align="center">

# ERPNext Open Banking

### Connect your bank accounts to ERPNext via [open-banking.io](https://open-banking.io)

**No eIDAS certificates required. No Plaid contract. From €3/month.**

</div>

---

## What it does

Syncs your bank transactions directly into ERPNext's **Bank Transaction** doctype,
where they appear in the **Bank Reconciliation Tool** — ready for matching against
payment entries and invoices.

- ✅ **No eIDAS/QWAC certificates** — open-banking.io handles PSD2 compliance
- ✅ **Zero-knowledge encryption** — your data is encrypted client-side; the service
  can't read it
- ✅ **~€3/month per account** — vs Plaid at $1,000–2,000/month
- ✅ **Self-hosted friendly** — works with Frappe Cloud or your own bench instance
- ✅ **Automated sync** — scheduler pulls new transactions every 6 hours

## How it works

```
open-banking.io API ──→ encrypted response ──→ ERPNext decrypts locally
                                                    ↓
                                              Bank Transaction doctype
                                                    ↓
                                         Bank Reconciliation Tool
```

The integration uses **server-to-server API** with zero-knowledge encryption:
the credentials bundle contains your private key, and all sensitive data
(account numbers, balances, transaction details) is decrypted locally inside
your ERPNext instance.

## Requirements

- ERPNext v15 (Frappe Framework v15) — tested; earlier versions are unsupported
- Python 3.10+ (Frappe v15 requirement)
- An [open-banking.io](https://open-banking.io) account with exported credentials bundle

## Installation

This app lives in the `erpnext/` subdirectory of the
[open-banking-io/clients](https://github.com/open-banking-io/clients) monorepo
(alongside the Python, Node, Rust, Go and other SDKs), so clone the monorepo
and point `bench get-app` at the subdirectory:

```bash
git clone https://github.com/open-banking-io/clients.git /tmp/obi-clients

# From your bench directory:
bench get-app /tmp/obi-clients/erpnext
bench --site <your-site> install-app erpnext_open_banking
bench --site <your-site> migrate
```

## Setup

### 1. Export your credentials from open-banking.io

Log in to [open-banking.io](https://open-banking.io), connect your banks, and
export the **credentials bundle** (`credentials.json`). It contains:
- `apiBaseUrl` — the API endpoint
- `apiKey` — your server-to-server API key
- `encryptionKey.privateKey` — your PKCS#8 private key (stays on your server)

### 2. Configure Open Banking Settings

Go to **Open Banking Settings** → paste the credentials bundle JSON → select your
default company → click **Test Connection**.

### 3. Create Connections

For each bank account you want to sync, create an **Open Banking Connection**:
- Select the OBI account ID
- Link it to the matching ERPNext **Bank Account**
- Set status to Active

### 4. Sync

Click **Sync from Open Banking** on any Bank Account, or let the scheduler run
automatically every 6 hours.

## Architecture

This app lives in the [open-banking-io/clients](https://github.com/open-banking-io/clients)
monorepo alongside the official SDKs (Node, Python, Rust, Go, Java, Ruby, PHP, .NET)
and the n8n community node. The envelope decryption code is vendored directly into
the app (no external runtime dependencies beyond what ERPNext already ships).

```
erpnext_open_banking/
├── hooks.py                          # Frappe hooks (scheduler, doctype JS)
├── erpnext_open_banking/
│   ├── doctype/
│   │   ├── open_banking_settings/    # Credentials bundle, company, sync config
│   │   ├── open_banking_connection/  # OBI account → ERPNext Bank Account mapping
│   │   └── open_banking_sync_log/    # Per-sync audit record
│   └── utils/
│       ├── envelope.py               # Zero-knowledge decryption (vendored)
│       ├── client.py                 # OBI API client (vendored, uses requests)
│       ├── mapper.py                 # OBI Transaction → Bank Transaction field map
│       └── connector.py              # Sync engine (fetch → map → dedup → insert)
└── public/js/bank_account.js         # "Sync from Open Banking" button
```

## Privacy & Security

Your private key **never leaves your ERPNext server**. The open-banking.io API
returns only encrypted envelopes that only your key can decrypt. The decryption
happens in-process using AES-256-GCM (ECDH P-256 + HKDF-SHA256).

## License

MIT © [open-banking.io](https://open-banking.io)
