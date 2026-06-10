# open-banking.io client SDKs

Server-to-server clients for [open-banking.io](https://open-banking.io) in **.NET, Node, Python,
Rust, Go, Java, Ruby, and PHP**.

open-banking.io is **zero-knowledge**: the service stores and returns only ciphertext it cannot read.
These SDKs do the two things an integrator needs — **authenticate** with your API key and **decrypt**
the data locally with your exported private key — and hand you clean, typed models.

| Language | Package | Path |
|---|---|---|
| .NET | `OpenBankingIO.Client` (NuGet) | [`dotnet/`](dotnet/) |
| Node / TypeScript | `@open-banking-io/client` (npm) | [`node/`](node/) |
| Python | `open-banking-io` (PyPI) | [`python/`](python/) |
| Rust | `open-banking-io` (crates.io) | [`rust/`](rust/) |
| Go | `github.com/open-banking-io/clients/go` | [`go/`](go/) |
| Java | `io.open-banking:open-banking-io-client` (Maven Central) | [`java/`](java/) |
| Ruby | `open-banking-io` (RubyGems) | [`ruby/`](ruby/) |
| PHP | `open-banking-io/client` (Packagist) | [`php/`](php/) |

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
```rust
// Rust
let client = OpenBankingClient::from_credentials("credentials.json")?;
for a in client.get_accounts()? {
    let booked = a.balances.iter().find(|b| b.type_ == "ITBD");
    println!("{:?} {:?} {}", a.iban, booked.map(|b| &b.amount), a.currency);
}
```
```go
// Go
client, _ := openbanking.FromCredentials("credentials.json", nil)
accounts, _ := client.GetAccounts()
for _, a := range accounts {
    for _, b := range a.Balances {
        if b.Type == "ITBD" { fmt.Println(a.Iban, b.Amount, a.Currency) }
    }
}
```
```java
// Java
var client = OpenBankingClient.fromCredentials("credentials.json");
for (Account a : client.getAccounts())
    a.balances().stream().filter(b -> b.type().equals("ITBD")).findFirst()
        .ifPresent(b -> System.out.println(a.iban() + " " + b.amount() + " " + a.currency()));
```
```ruby
# Ruby
client = OpenBankingIO::Client.from_credentials("credentials.json")
client.get_accounts.each do |a|
  booked = a.balances.find { |b| b.type == "ITBD" }
  puts "#{a.iban} #{booked&.amount} #{a.currency}"
end
```
```php
// PHP
$client = OpenBankingIO\Client::fromCredentials("credentials.json");
foreach ($client->getAccounts() as $a) {
    foreach ($a->balances as $b) {
        if ($b->type === "ITBD") echo "{$a->iban} {$b->amount} {$a->currency}\n";
    }
}
```

All eight expose the same surface: `getAccounts`, `getTransactions(accountId, …)`, `getConnections`,
`sync(accountId)`, `syncAll()`. Sync decrypts the account's session uid locally and posts it, so the
service can refresh from the bank without ever holding it in plaintext.

## The encryption scheme

Each sensitive value is an **envelope**: `version(1) | ephemeralPublicKey(65) | nonce(12) | tag(16) |
ciphertext`, produced with **ephemeral ECDH on P-256 → HKDF-SHA256 (salt = 32 zero bytes, info =
`bank.core.ci/zk/v1`) → AES-256-GCM**. Only your private key can open it. All eight SDKs are verified
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
cd rust   && cargo test
cd go     && go test ./...
cd java   && mvn -B verify
cd ruby   && bundle install && bundle exec rspec
cd php    && composer install && vendor/bin/phpunit
```

CI (`.github/workflows/ci.yml`) builds and tests all eight on every push. **Releases are per-package**:
each package publishes from its own `<dir>/vX.Y.Z` tag (e.g. `node/v0.2.0`), so tagging one package never
republishes the others. Cut a release from **Actions → Release** (pick a package + version) — see
[`RELEASING.md`](RELEASING.md). Targets: NuGet, npm, PyPI, crates.io, Maven Central, RubyGems, Packagist,
and the Go module proxy.

MIT licensed.
