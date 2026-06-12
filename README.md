<p align="center">
  <a href="https://open-banking.io">
    <img src="https://raw.githubusercontent.com/open-banking-io/clients/main/.github/logo.png" alt="open-banking.io" height="56">
  </a>
</p>

# open-banking.io client SDKs

[![CI](https://github.com/open-banking-io/clients/actions/workflows/ci.yml/badge.svg)](https://github.com/open-banking-io/clients/actions/workflows/ci.yml)
[![Semgrep](https://github.com/open-banking-io/clients/actions/workflows/semgrep.yml/badge.svg)](https://github.com/open-banking-io/clients/actions/workflows/semgrep.yml)
[![Gitleaks](https://github.com/open-banking-io/clients/actions/workflows/gitleaks.yml/badge.svg)](https://github.com/open-banking-io/clients/actions/workflows/gitleaks.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/open-banking-io/clients/badge)](https://securityscorecards.dev/viewer/?uri=github.com/open-banking-io/clients)
[![codecov](https://codecov.io/gh/open-banking-io/clients/branch/main/graph/badge.svg)](https://codecov.io/gh/open-banking-io/clients)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

Official server-to-server SDKs for **[open-banking.io](https://open-banking.io)** in .NET, Node,
Python, Rust, Go, Java, Ruby, and PHP.

open-banking.io is **zero-knowledge**: the service stores and returns only ciphertext it cannot read.
Each SDK does the two things an integrator needs — **authenticate** with your API key and **decrypt**
your data locally with your private key — and hands you clean, typed models.

> **New here?** Create an account and export your credentials bundle at
> **[open-banking.io](https://open-banking.io)**, then pick your language below.

## Pick your language

Each package has its own README with install instructions, a runnable example, and language-specific
notes (money types, async, dependencies).

| Language | Package | Version | Guide |
|---|---|---|---|
| .NET | [`OpenBankingIO.Client`](https://www.nuget.org/packages/OpenBankingIO.Client) (NuGet) | [![NuGet](https://img.shields.io/nuget/v/OpenBankingIO.Client?logo=nuget&label=)](https://www.nuget.org/packages/OpenBankingIO.Client) | [`dotnet/`](dotnet/src/OpenBankingIO.Client/README.md) |
| Node / TypeScript | [`@open-banking-io/client`](https://www.npmjs.com/package/@open-banking-io/client) (npm) | [![npm](https://img.shields.io/npm/v/%40open-banking-io%2Fclient?logo=npm&label=)](https://www.npmjs.com/package/@open-banking-io/client) | [`node/`](node/) |
| Python | [`open-banking-io`](https://pypi.org/project/open-banking-io/) (PyPI) | [![PyPI](https://img.shields.io/pypi/v/open-banking-io?logo=pypi&logoColor=white&label=)](https://pypi.org/project/open-banking-io/) | [`python/`](python/) |
| Rust | [`open-banking-io`](https://crates.io/crates/open-banking-io) (crates.io) | [![crates.io](https://img.shields.io/crates/v/open-banking-io?logo=rust&label=)](https://crates.io/crates/open-banking-io) | [`rust/`](rust/) |
| Go | [`github.com/open-banking-io/clients/go`](https://pkg.go.dev/github.com/open-banking-io/clients/go) | [![Go Reference](https://pkg.go.dev/badge/github.com/open-banking-io/clients/go.svg)](https://pkg.go.dev/github.com/open-banking-io/clients/go) | [`go/`](go/) |
| Java | [`io.open-banking:open-banking-io-client`](https://central.sonatype.com/artifact/io.open-banking/open-banking-io-client) (Maven Central) | [![Maven Central](https://img.shields.io/maven-central/v/io.open-banking/open-banking-io-client?logo=apachemaven&label=)](https://central.sonatype.com/artifact/io.open-banking/open-banking-io-client) | [`java/`](java/) |
| Ruby | [`open-banking-io`](https://rubygems.org/gems/open-banking-io) (RubyGems) | [![Gem](https://img.shields.io/gem/v/open-banking-io?logo=rubygems&label=)](https://rubygems.org/gems/open-banking-io) | [`ruby/`](ruby/) |
| PHP | [`open-banking-io/client`](https://packagist.org/packages/open-banking-io/client) (Packagist) | [![Packagist](https://img.shields.io/packagist/v/open-banking-io/client?logo=packagist&logoColor=white&label=)](https://packagist.org/packages/open-banking-io/client) | [`php/`](php/) |

Prefer the terminal? There's also a [command-line client](cli/) (`openbanking`).

## Getting started

1. **Get your credentials.** In the [open-banking.io](https://open-banking.io) app, export your
   **credentials bundle** (`credentials.json`) — it contains your `apiBaseUrl`, an **API key**, and
   your **encryption private key** (PKCS#8).
2. **Point an SDK at the bundle.** Every request sends `X-Api-Key`; every response is decrypted
   in-process with your private key. The service never sees your plaintext data.

```ts
// Node / TypeScript — the same shape exists in all eight languages (see each guide above).
import { OpenBankingClient } from "@open-banking-io/client";

const client = OpenBankingClient.fromCredentials("credentials.json");
for (const a of await client.getAccounts()) {
  const booked = a.balances.find((b) => b.type === "ITBD");
  console.log(`${a.iban}: ${booked?.amount} ${a.currency}`);
}
```

<details>
<summary>The same example in .NET, Python, Rust, Go, Java, Ruby, and PHP</summary>

```csharp
// .NET
using var client = OpenBankingClient.FromCredentials("credentials.json");
foreach (var a in await client.GetAccountsAsync())
    Console.WriteLine($"{a.Iban}: {a.Balances.First(b => b.Type == "ITBD").Amount} {a.Currency}");
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
</details>

All eight expose the same surface — `getAccounts`, `getTransactions(accountId, …)`, `getConnections`,
`sync(accountId)`, `syncAll()`. `sync` decrypts the account's session uid locally and posts it, so the
service can refresh from the bank without ever holding it in plaintext.

## How it works

1. Your bank credentials are never sent to open-banking.io.
2. Your account data (IBAN, balances, transactions) is stored **encrypted** by the service.
3. Only your **local private key** can decrypt it — the service holds ciphertext it cannot read.

Each sensitive value is an **envelope** sealed with **ephemeral ECDH (P-256) → HKDF-SHA256 →
AES-256-GCM**; only your private key can open it, and all eight SDKs are verified against the same
`fixtures/` so they decrypt identically. The full wire format, trust boundaries, and assumptions are
in **[`THREAT_MODEL.md`](THREAT_MODEL.md)**.

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

CI builds and tests all eight on every push. **Releases are per-package** — each publishes from its own
`<dir>/vX.Y.Z` tag (e.g. `node/v0.2.0`), so tagging one package never republishes the others. Cut a
release from **Actions → Release** (pick a package + version); see [`RELEASING.md`](RELEASING.md).

## Security & contributing

Zero-knowledge security model and trust boundaries: [`THREAT_MODEL.md`](THREAT_MODEL.md). Report
vulnerabilities privately per [`SECURITY.md`](SECURITY.md). Contributions welcome — see
[`CONTRIBUTING.md`](CONTRIBUTING.md), [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md), and
[`SUPPORT.md`](SUPPORT.md).

MIT licensed.
