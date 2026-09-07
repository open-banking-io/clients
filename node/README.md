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

Partners let their users connect banks through open-banking.io — a standard authorization-code
flow with PKCE and `form_post`, documented at
[open-banking.io/en/docs/partners](https://open-banking.io/en/docs/partners). You hold your own
**decryption key**: every user who connects through your client is encrypted to it, and you
decrypt their data with the one private half your deployment loads. The service keeps only the
public half and cannot recover the private one. Install it first — until you do, a Connect
client cannot be registered and every authorization answers `temporarily_unavailable`.

```ts
import {
  answerRecipientKeyChallenge,
  generateRecipientKeyPair,
  recipientKeyFingerprint,
  recipientPublicKey,
} from "@open-banking-io/client";

// 0. Once: generate the pair, keep privateKeyPkcs8Base64 in your secret store, and install
//    publicKeyRawBase64 on your partner page (https://open-banking.io/app/partner#decryption-key
//    does all of this in the browser, if you prefer). Print the fingerprint at boot and compare it
//    with the one the partner page shows after every deploy.
const pair = await generateRecipientKeyPair();
console.log(recipientKeyFingerprint(pair.publicKeyRawBase64));
// Bringing a key generated elsewhere? The partner page hands you an envelope to prove you hold it:
const answer = await answerRecipientKeyChallenge(RECIPIENT_PRIVATE_KEY, envelope);
```

The flow itself: keep the client secret and the verifier on your server.

```ts
import {
  buildAuthorizeUrl,
  createPkce,
  createState,
  discover,
  exchangeCode,
  parseRelay,
  OpenBankingClient,
  RelayError,
} from "@open-banking-io/client";

const ISSUER = "https://open-banking.io";

// 1. Start: keep the verifier server-side, keyed by state, and send the browser to the URL.
//    challenge: "pin_code" gives the user a typed code instead of a magic link — recommended for
//    every journey, and required for a popup.
app.get("/connect", async (req, res) => {
  const pkce = createPkce();
  const state = createState();
  const mode = req.query.mode === "redirect" ? "redirect" : "popup";
  await flows.put(state, {
    state,
    verifier: pkce.verifier,
    sessionId: req.session.id,
    mode,
    expiresAt: Date.now() + 45 * 60_000,
  });
  res.redirect(
    buildAuthorizeUrl({
      issuer: ISSUER,
      clientId: CLIENT_ID,
      redirectUri: `${SELF_URL}/callback`,
      state,
      codeChallenge: pkce.challenge,
      challenge: "pin_code",
    }),
  );
});

// 2. Callback: the consent page form-posts code, state, iss and (empty) privateKey/publicKey — or
//    error=access_denied when the user went back to you. Consume the flow by state first, so a
//    cancel consumes it too; the lookup is what binds the POST to the session that started it.
app.post("/callback", express.urlencoded({ extended: false }), async (req, res) => {
  const { issuer } = await discover(ISSUER);
  const flow = await flows.take(req.body.state);
  if (!flow || flow.expiresAt < Date.now()) return res.status(400).send("unknown or expired state");
  let relay;
  try {
    relay = parseRelay(req.body, {
      expectedState: flow.state,
      issuer,
      expectPrivateKey: "optional",
    });
  } catch (e) {
    if (e instanceof RelayError && e.code === "access_denied")
      return finish(res, flow, "cancelled");
    if (e instanceof RelayError && e.reason) {
      // partner_key_missing or partner_suspended: your side, not the bank's — install the key on
      // your partner page, or write to us. The user only sees "unavailable".
      console.error("connect refused:", e.reason);
      return finish(res, flow, "unavailable");
    }
    throw e;
  }
  const token = await exchangeCode({
    issuer,
    clientId: CLIENT_ID,
    clientSecret: CLIENT_SECRET,
    code: relay.code,
    codeVerifier: flow.verifier,
    redirectUri: `${SELF_URL}/callback`,
  });
  // One key per deployment, so store the token alone. A user who connected before your key was
  // installed still relays their own private key — keep it with their token when present.
  await bundles.put(flow.sessionId, { token, legacyPrivateKey: relay.privateKey || undefined });
  finish(res, flow, "connected");
});

// A redirect-mode flow lands back on your page; a popup signals its opener and closes.
const finish = (res, flow, outcome) =>
  flow.mode === "redirect"
    ? res.redirect(302, `/?connect=${outcome}`)
    : res.send(closePage(outcome));
const closePage = (
  outcome,
) => `<!doctype html><p>${outcome === "connected" ? "Connected — you can close this window." : "Cancelled."}</p>
<script>try{new BroadcastChannel("bank-connect").postMessage(${JSON.stringify(outcome)})}catch{}setTimeout(()=>window.close(),300)</script>`;

// 3. Read (inside any handler that has the stored token): the token plus your decryption key —
//    or the user's own key, for the few who connected before you had one — is a complete bundle.
const { token, legacyPrivateKey } = await bundles.get(req.session.id);
const client = OpenBankingClient.fromTokenResponse(
  token,
  legacyPrivateKey ?? RECIPIENT_PRIVATE_KEY,
);
const accounts = await client.getAccounts();
```

`parseRelay` throws a `RelayError` (`access_denied` — the user cancelled, a normal outcome —
`oauth_error`, `state_mismatch`, `issuer_mismatch`, `missing_code`, `missing_private_key`) and
compares `state` and `iss` in constant time. On an `oauth_error`, `reason` is `partner_key_missing`
or `partner_suspended` when the server said so in as many words. `exchangeCode`, `revokeToken` and
`userinfo` throw an `OAuthError` carrying the RFC 6749 `error` and `error_description`. An
`invalid_grant` is terminal for that code — restart the flow. A flow is valid for 45 minutes on the
server; keep your own `state` at least that long.

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
