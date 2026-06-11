<p align="center">
  <a href="https://open-banking.io">
    <img src="https://raw.githubusercontent.com/open-banking-io/clients/main/.github/logo.png" alt="open-banking.io" height="56">
  </a>
</p>

# @open-banking-io/client (Node / TypeScript)

Server-to-server client for [open-banking.io](https://open-banking.io). It authenticates with your
**API key** and decrypts the **zero-knowledge** data envelopes locally with your exported **private
key** — the service only ever returns ciphertext it cannot read.

```bash
npm install @open-banking-io/client
```

Requires Node >= 20 (uses the built-in `node:crypto` WebCrypto and `fetch`; no runtime deps).

```ts
import { OpenBankingClient } from "@open-banking-io/client";

// Load the credentials .json you exported from the app (API key + private key).
const client = OpenBankingClient.fromCredentials("credentials.json");

for (const account of await client.getAccounts()) {
  const booked = account.balances.find((b) => b.type === "ITBD");
  console.log(
    `${account.displayName ?? account.ownerName} ${account.iban}: ${booked?.amount} ${account.currency}`,
  );

  const page = await client.getTransactions(account.id, { limit: 50 });
  for (const t of page.items) {
    console.log(`  ${t.bookingDate}  ${t.creditorName ?? t.debtorName}  ${t.amount} ${t.currency}`);
  }
}

// Trigger an online sync (decrypts the account uid locally and posts it):
await client.sync(accountId);
```

Or construct it explicitly:

```ts
const client = new OpenBankingClient({ apiBaseUrl, apiKey, privateKeyPkcs8 });
```

Every request carries a `User-Agent: open-banking-io/node/<version>` header and a default 30s timeout
(override via the `timeoutMs` option) so a hung connection can't block forever.

## Money

Amounts (`balance.amount`, `transaction.amount`, `transaction.balanceAfterTransaction`) are exposed
as **decimal strings** and never parsed to floats — keep them as strings or feed them into a decimal
library to avoid precision loss.

## Encryption

Envelopes use **ECDH P-256 → HKDF-SHA256 → AES-256-GCM**. Decryption requires the private key from
your credentials bundle and happens entirely in-process. See the
[repo README](https://github.com/open-banking-io/clients) for the full scheme and the other language
clients (.NET, Python).

MIT licensed.
