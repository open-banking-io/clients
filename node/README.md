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

## Partner Connect (OAuth 2.0 + PKCE)

Partners let their users connect banks through open-banking.io and receive a delegated key plus the
user's private key — a standard authorization-code flow with PKCE and `form_post`, documented at
[open-banking.io/en/docs/partners](https://open-banking.io/en/docs/partners). The `connect` helpers
cover every step; keep the client secret and the verifier on your server.

```ts
import {
  buildAuthorizeUrl,
  createPkce,
  createState,
  discover,
  exchangeCode,
  parseRelay,
  OpenBankingClient,
} from "@open-banking-io/client";

const ISSUER = "https://open-banking.io";

// 1. Start: keep the verifier server-side, keyed by state, and send the browser to the URL.
app.get("/connect", async (req, res) => {
  const pkce = createPkce();
  const state = createState();
  await flows.put(state, { verifier: pkce.verifier, sessionId: req.session.id });
  res.redirect(
    buildAuthorizeUrl({
      issuer: ISSUER,
      clientId: CLIENT_ID,
      redirectUri: `${SELF_URL}/callback`,
      state,
      codeChallenge: pkce.challenge,
      challenge: "pin_code", // popup mode; omit for a redirect flow
    }),
  );
});

// 2. Callback: the consent page form-posts code, state, iss, privateKey and publicKey.
app.post("/callback", async (req, res) => {
  const { issuer } = await discover(ISSUER);
  const flow = await flows.take(req.body.state);
  const relay = parseRelay(req.body, { expectedState: flow?.state ?? "", issuer });
  const token = await exchangeCode({
    issuer,
    clientId: CLIENT_ID,
    clientSecret: CLIENT_SECRET,
    code: relay.code,
    codeVerifier: flow.verifier,
    redirectUri: `${SELF_URL}/callback`,
  });
  await bundles.put(flow.sessionId, { token, privateKey: relay.privateKey });
  res.render("close");
});

// 3. Read: the token plus the relayed private key is a complete credentials bundle.
const client = OpenBankingClient.fromTokenResponse(token, privateKey);
const accounts = await client.getAccounts();
```

`parseRelay` throws a `RelayError` (`oauth_error`, `state_mismatch`, `issuer_mismatch`,
`missing_code`, `missing_private_key`) and compares `state` and `iss` in constant time;
`exchangeCode`, `revokeToken` and `userinfo` throw an `OAuthError` carrying the RFC 6749 `error`
and `error_description`. An `invalid_grant` is terminal for that code — restart the flow.

## Money

Amounts (`balance.amount`, `transaction.amount`, `transaction.balanceAfterTransaction`) are exposed
as **decimal strings** and never parsed to floats — keep them as strings or feed them into a decimal
library to avoid precision loss.

## Encryption

Envelopes use **ECDH P-256 → HKDF-SHA256 → AES-256-GCM**, implemented with the built-in `node:crypto`
WebCrypto. Decryption requires the private key from your credentials bundle and happens entirely
in-process. Full wire format and the other language clients:
[repo README](https://github.com/open-banking-io/clients) ·
[`THREAT_MODEL.md`](https://github.com/open-banking-io/clients/blob/main/THREAT_MODEL.md).

MIT licensed.
