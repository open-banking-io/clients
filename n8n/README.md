# n8n-nodes-open-banking-io

An [n8n](https://n8n.io) community node for [open-banking.io](https://open-banking.io).

Pull your bank **accounts**, **balances** and **transactions** into n8n workflows.
All sensitive data is decrypted **locally inside your n8n instance** using your
exported private key — the open-banking.io service only ever returns ciphertext it
cannot read (zero-knowledge), and your private key never leaves n8n or goes on the wire.

This package has **zero runtime dependencies**: decryption uses the Web Crypto API
built into Node, so it qualifies as a verified n8n community node.

## Nodes

- **Open Banking IO** (action) — `Account: Get Many`, `Transaction: Get Many`
  (date range + auto-pagination via *Return All*), `Connection: Get Many`,
  `Bank: Get Many`, `Sync: Account` / `Sync: All`.
- **Open Banking IO Trigger** (polling) — fires when new transactions arrive across
  all your accounts, remembering the last one seen between polls.

## Installation

### Self-hosted (any n8n)

In n8n, go to **Settings → Community Nodes → Install** and enter:

```
n8n-nodes-open-banking-io
```

### Manual / development

```bash
npm install
npm run build
# then link into your n8n custom extensions folder (~/.n8n/custom)
```

## Credentials

Create an **Open Banking IO API** credential and paste the **credentials bundle JSON**
you exported from open-banking.io (or the CLI). It contains `apiBaseUrl`, `apiKey`
and `encryptionKey.privateKey`. Click **Test** to verify it can reach the API.

## Usage notes

- Monetary amounts are returned as **decimal strings** (e.g. `"828.13"`) — never as
  floats — so you don't lose precision. Convert deliberately if you need arithmetic.
- *Transaction → Get Many* with **Return All** enabled pages through the whole
  statement automatically; otherwise it returns up to **Limit** rows.
- The Trigger backfills *Initial Lookback (Days)* on its first run, then only emits
  transactions newer than the last one it saw.

## License

[MIT](LICENSE)
