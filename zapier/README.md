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

Once published, search for **open-banking.io** in the
[Zapier app directory](https://zapier.com/apps) and connect it from any Zap —
no installation step is needed for users.

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
npm test              # mocha tests (schema validation, envelope roundtrip, client parsing, trigger cursor)

npm install -g zapier-platform-cli
zapier validate       # full Zapier platform validation
zapier push           # deploy (needs `zapier login` or ZAPIER_DEPLOY_KEY, plus a committed .zapierapprc)
```

Releases are tag-driven: pushing `zapier/vX.Y.Z` in the monorepo tests the
package, subtree-splits `zapier/` to the `open-banking-io/zapier` mirror, and
the mirror's publish workflow runs `zapier push`. The one-time `zapier register`
/ `zapier link` must be done locally and the resulting `.zapierapprc` committed
here (it holds only the app id — not a secret).

## License

[MIT](LICENSE)
