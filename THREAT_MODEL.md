# Threat Model

`open-banking-io` is the **client** side of a zero-knowledge banking model. This document states what the
SDKs protect against and what they assume.

## The zero-knowledge model

1. Your bank credentials are never sent to open-banking.io.
2. Your account data (IBAN, balances, transactions) is stored **encrypted** by the service.
3. Only your **local private key** can decrypt it — the service holds ciphertext it cannot read.

These SDKs implement the client: they hold the private key and decrypt envelopes in-process.

## Encryption scheme

Each sensitive value is an **envelope**:

```
version(1)=0x01 | ephemeralPublicKey(65) | nonce(12) | tag(16) | ciphertext
```

It is produced with **ephemeral ECDH on P-256 → HKDF-SHA256** (salt = 32 zero bytes, info =
`bank.core.ci/zk/v1`) **→ AES-256-GCM**. Only your private key can open it. Every SDK validates that
the ephemeral public key is a valid point on P-256, and AES-GCM tag verification rejects tampered
ciphertext. All eight SDKs are verified against the **same fixtures** (`fixtures/`) so they decrypt
identically and interoperate with the live service's wire format.

## Trust boundaries

**The SDK trusts:**
- The **credentials bundle** (`apiBaseUrl`, `apiKey`, `privateKeyPkcs8`) you provide — assumed to come
  unmodified from the official app.
- The **API endpoint** to be the genuine service, reached over TLS.

**The SDK does NOT trust:**
- The **open-banking.io service** — even if compromised it sees only ciphertext, never your plaintext data.
- The **network** — TLS protects data in transit; a passive MITM still only sees ciphertext.

## Scenarios & mitigations

| Threat | Mitigation |
|---|---|
| **Stolen credentials bundle** | Rotate the API key in the app. Treat the bundle like a password — store it in a secrets manager, never in version control. The private key cannot be rotated retroactively, so a leaked key compromises previously-stored envelopes. |
| **MITM on the network** | SDKs use HTTPS and verify certificates by default; intercepted responses are ciphertext only. |
| **Compromised local host** | Out of the SDK's control — malware with memory access can read anything. Run on trusted hardware; the SDK never persists the private key to disk on your behalf. Rust additionally zeroizes the derived key material after use. |
| **Forged / off-curve ephemeral key** | Every SDK validates that the ephemeral public key is a valid point on P-256 and rejects malformed envelopes; AES-GCM tag verification rejects tampered ciphertext. |
| **Rate limiting / abuse** | The API enforces limits and requires a valid key; revoke the key if leaked. |

## Session handling

`sync(accountId)` decrypts the account's session **uid locally** and posts it, so the service can refresh
from the bank **without ever holding the uid in plaintext**. Sessions are time-limited (see
`Connection.validUntil` / `needsReconnect`).

## CLI onboarding (browser-relay)

`openbanking login` sets the CLI up in one step without the private key ever reaching the server. The CLI
starts a localhost loopback and opens the browser to the app's authorize page; you sign in and enter your
passphrase **in the browser**, which unlocks the locally-held private key and POSTs the full credentials
**directly to `http://127.0.0.1:<port>`** — browser → localhost only. The server never sees the private key
(not even encrypted) or the passphrase. The relay is bound by three things: the random loopback port, the
PKCE `verifier` (held only by the CLI, required to redeem the one-time code for the API key), and a `state`
nonce echoed by the authenticated browser. CORS on the loopback is scoped to the app origin. The CLI writes
`credentials.json` at `0600` and never logs the key or passphrase.

## Assumptions & limitations

- The private key staying private is load-bearing — if it leaks, future envelopes encrypted to its public
  counterpart are compromised.
- The SDK does not pin certificates beyond the platform's default TLS validation.
- The credentials bundle is trusted as-is (no signature verification of the bundle itself).

Report security concerns per [`SECURITY.md`](SECURITY.md).
