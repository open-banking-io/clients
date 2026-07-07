<p align="center">
  <a href="https://open-banking.io">
    <img src="https://raw.githubusercontent.com/open-banking-io/clients/main/.github/logo.png" alt="open-banking.io" height="56">
  </a>
</p>

# open-banking.io Zapier App

A [Zapier](https://zapier.com) integration for [open-banking.io](https://open-banking.io).

Pull your bank **accounts**, **balances** and **transactions** into Zapier workflows.
All sensitive data is decrypted **locally inside your Zapier connection** using your
exported private key — the open-banking.io service only ever returns ciphertext it
cannot read (zero-knowledge), and your private key never goes on the wire.

This app has **zero runtime dependencies** beyond `zapier-platform-core`: decryption
uses the Web Crypto API built into Node, with no external crypto libraries.

## Features

- **Trigger** — *New Transaction*: fires when new bank transactions arrive across
  all your accounts, remembering the last one seen between polls.
- **Searches** — *Find Account*: list all accounts with decrypted balances;
  *Find Transactions*: list an account statement with date-range filtering.
- **Creates** — *Sync Account*: trigger a fresh data pull from the bank.

## Installation

Install from the Zapier app directory (once published), or via the CLI:

```bash
npm install -g zapier-platform-cli
zapier install open-banking-io
```

## Credentials

When connecting your account, paste the **credentials bundle JSON** you exported
from open-banking.io (or the CLI). It contains `apiBaseUrl`, `apiKey` and
`encryptionKey.privateKey`. Click **Test** to verify the connection works.

Optionally set **API Base URL Override** to point at a different environment
(e.g. `https://api.staging.open-banking.io` or a local instance) without
re-exporting a bundle. Leave it empty to use the `apiBaseUrl` from the bundle.

## Usage notes

- Monetary amounts are returned as **decimal strings** (e.g. `"828.13"`) — never as
  floats — so you don't lose precision. Convert deliberately if you need arithmetic.
- The *New Transaction* trigger backfills *Initial Lookback (Days)* on its first
  run, then only emits transactions newer than the last one it saw.
- *Find Transactions* supports `from`/`to` date filters and a `limit`.

## Development

```bash
npm install
npm test              # mocha tests (envelope roundtrip, client parsing, pagination, trigger cursor)
zapier test           # full Zapier platform validation
zapier push           # deploy to Zapier (needs ZAPIER_DEPLOY_KEY)
```

## License

[MIT](LICENSE)
