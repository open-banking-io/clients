# open-banking.io client SDKs

Server-to-server clients for [open-banking.io](https://open-banking.io) in **.NET, Node, and Python**.

open-banking.io is **zero-knowledge**: the service stores and returns only ciphertext it cannot read.
These SDKs do the two things an integrator needs — **authenticate** with your API key and **decrypt**
the data locally with your exported private key — and hand you clean, typed models.

| Language | Package | Path |
|---|---|---|
| .NET | `OpenBankingIO.Client` (NuGet) | [`dotnet/`](dotnet/) |
| Node / TypeScript | `@open-banking-io/client` (npm) | [`node/`](node/) |
| Python | `open-banking-io` (PyPI) | [`python/`](python/) |

## How it works

1. In the app, export your **credentials bundle** (`.json`) — it contains your `apiBaseUrl`, an
   **API key**, and your **encryption private key** (PKCS#8).
2. Point an SDK at the bundle. Every request sends `X-Api-Key`; every response is decrypted in-process.

```csharp
// .NET
using var client = OpenBankingClient.FromCredentials("credentials.json");
foreach (var a in await client.GetAccountsAsync())
    Console.WriteLine($"{a.Iban}: {a.Balances.First(b => b.Type == "ITBD").Amount} {a.Currency}");
```
```ts
// Node
const client = OpenBankingClient.fromCredentials("credentials.json");
for (const a of await client.getAccounts())
  console.log(a.iban, a.balances.find(b => b.type === "ITBD")?.amount, a.currency);
```
```python
# Python
client = OpenBankingClient.from_credentials("credentials.json")
for a in client.get_accounts():
    booked = next(b for b in a.balances if b.type == "ITBD")
    print(a.iban, booked.amount, a.currency)
```

All three expose the same surface: `getAccounts`, `getTransactions(accountId, …)`, `getConnections`,
`sync(accountId)`, `syncAll()`. Sync decrypts the account's session uid locally and posts it, so the
service can refresh from the bank without ever holding it in plaintext.

## The encryption scheme

Each sensitive value is an **envelope**: `version(1) | ephemeralPublicKey(65) | nonce(12) | tag(16) |
ciphertext`, produced with **ephemeral ECDH on P-256 → HKDF-SHA256 (salt = 32 zero bytes, info =
`bank.core.ci/zk/v1`) → AES-256-GCM**. Only your private key can open it. The three SDKs are verified
against the **same fixtures** (`fixtures/`) so they decrypt identically and interoperate with the live
service's wire format.

## Development

```bash
# regenerate the shared test fixtures (keypair + encrypted sample responses)
node tools/generate-fixtures.mjs

# run each SDK's tests (crypto round-trip + a mock-server integration suite)
dotnet test dotnet/
cd node   && npm install && npm test
cd python && pip install -e .[dev] && pytest -q
```

CI (`.github/workflows/ci.yml`) builds and tests all three on every push. The `publish-*.yml` workflows
publish to NuGet / npm / PyPI on a `v*` tag — add the registry secrets (`NUGET_API_KEY`, `NPM_TOKEN`,
`PYPI_API_TOKEN`) to enable them.

MIT licensed.
