<p align="center">
  <a href="https://open-banking.io">
    <img src="https://raw.githubusercontent.com/open-banking-io/clients/main/.github/logo.png" alt="open-banking.io" height="56">
  </a>
</p>

# open-banking-io (PHP)

Server-to-server client for [open-banking.io](https://open-banking.io). It authenticates with your
**API key** and decrypts the **zero-knowledge** data envelopes locally with your exported **private
key** — the service only ever returns ciphertext it cannot read.

```bash
composer require open-banking-io/client
```

Requires PHP **8.1+** with `ext-openssl`, `ext-curl` and `ext-json` (no runtime Composer dependencies).

Every request carries a `User-Agent: open-banking-io/php/<version>` header (`Client::VERSION`) and uses a 30s total / 10s connect timeout.

```php
use OpenBankingIO\Client;

// Load the credentials .json you exported from the app (API key + private key).
$client = Client::fromCredentials('credentials.json');

foreach ($client->getAccounts() as $account) {
    $booked = null;
    foreach ($account->balances as $b) {
        if ($b->type === 'ITBD') {
            $booked = $b;
        }
    }
    $label = $account->displayName ?? $account->ownerName;
    printf("%s %s: %s %s\n", $label, $account->iban, $booked?->amount, $account->currency);

    $page = $client->getTransactions($account->id, ['limit' => 50]);
    foreach ($page->items as $t) {
        printf("  %s  %s  %s %s\n", $t->bookingDate, $t->creditorName ?? $t->debtorName, $t->amount, $t->currency);
    }

    // Trigger an online sync (decrypts the account uid locally and posts it):
    $client->sync($account->id);
}
```

Or construct it explicitly:

```php
$client = new Client($apiBaseUrl, $apiKey, $privateKeyPkcs8);
```

### Custom transport & timeouts

Both the constructor and `Client::fromCredentials()` take an optional `array $options` for
proxy, custom CA / mTLS, and timeout control. `curl_options` (a map of `CURLOPT_* => value`) is
applied **last**, so it wins over the SDK defaults; `timeout` / `connect_timeout` (seconds)
override the 30s / 10s defaults:

```php
$client = new Client($apiBaseUrl, $apiKey, $privateKeyPkcs8, [
    'timeout' => 60,          // total request timeout (seconds)
    'connect_timeout' => 5,   // connection-establishment timeout (seconds)
    'curl_options' => [
        CURLOPT_PROXY  => 'http://proxy.internal:8080',
        CURLOPT_CAINFO => '/etc/ssl/corp-ca.pem',
    ],
]);

// Same options are accepted by the bundle loader:
$client = Client::fromCredentials('credentials.json', ['timeout' => 60]);
```

## API

- `getAccounts(): Account[]` — decrypts each account's envelope, display name and balances.
- `getTransactions(string $accountId, array $opts = []): TransactionPage` — `$opts` keys: `from`, `to`, `limit`, `offset`.
- `getConnections(): Connection[]`
- `sync(string $accountId): SyncResult` — decrypts the account uid locally and posts it; throws if the account has no active session.
- `syncAll(): SyncAllResult` — syncs every account that has an active session.

Money/amount fields are exposed as **decimal `string`s** (exact; never a float). Models are
`final` classes with `readonly` public properties under `OpenBankingIO\Model`.

## Encryption

Envelopes use **ECDH P-256 → HKDF-SHA256 → AES-256-GCM** and are decrypted entirely in-process with
`ext-openssl`. Full wire format and the other language clients:
[repo README](https://github.com/open-banking-io/clients) ·
[`THREAT_MODEL.md`](https://github.com/open-banking-io/clients/blob/main/THREAT_MODEL.md).

## Development

```bash
composer install
vendor/bin/phpunit
```

The tests read the shared fixtures at the repo-root `fixtures/` directory. The integration test
spins up a local mock API using PHP's built-in server (`php -S`) as a subprocess.

### Static analysis & formatting

```bash
vendor/bin/phpstan analyse                       # PHPStan level max (src/ + tests/)
vendor/bin/php-cs-fixer fix --dry-run --diff     # PSR-12 check (drop --dry-run to apply)
```

### Coverage

Coverage needs a driver. CI runs with **pcov**; locally you can also use Xdebug via
`XDEBUG_MODE=coverage`. The `<coverage>` config emits Cobertura plus a text summary:

```bash
# CI / pcov:
php -d pcov.enabled=1 vendor/bin/phpunit --coverage-cobertura=coverage/cobertura.xml
# or with Xdebug:
XDEBUG_MODE=coverage vendor/bin/phpunit --coverage-cobertura=coverage/cobertura.xml
```

The report is written to `coverage/cobertura.xml` (gitignored). Because `phpunit.xml` enables a
`<coverage>` report and `failOnWarning`, run the suite **with a coverage driver present**
(pcov or `XDEBUG_MODE=coverage`); otherwise PHPUnit emits a "no coverage driver" warning.

## Publishing (monorepo caveat)

PHP packages are distributed through [Packagist](https://packagist.org), which auto-syncs from
GitHub when a new tag is pushed. **Packagist expects `composer.json` at a repository root**, but this
package lives in the `php/` subdirectory of a monorepo. Two ways to publish it:

1. **Subtree mirror (recommended):** publish `php/` to a dedicated mirror repo, e.g.
   `git subtree split --prefix=php -b php-release && git push <mirror> php-release:main`, and register
   that mirror on Packagist.
2. **VCS config pointing at the path:** some Packagist setups can be configured to read a package
   from a subdirectory — this is not the default and may require a custom/Private Packagist config.

This is intentionally **not solved here** — the `publish-php.yml` workflow validates the manifest and
runs the tests, then optionally pings Packagist's update API when the
`PACKAGIST_USERNAME`/`PACKAGIST_API_TOKEN` secrets are present.

MIT licensed.
