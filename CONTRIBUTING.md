# Contributing

Thanks for your interest! This monorepo contains eight client SDKs that all implement the same
zero-knowledge contract and are verified against **shared test fixtures**.

## Layout

`dotnet/ node/ python/ rust/ go/ java/ ruby/ php/` — one SDK each · `fixtures/` — shared, language-agnostic
test vectors · `tools/generate-fixtures.mjs` — regenerates them · `.github/workflows/` — CI, coverage, and
per-package publish.

## Local development

Each SDK is self-contained. Generate fixtures once, then run the language(s) you touched:

```bash
node tools/generate-fixtures.mjs              # shared crypto test vectors

dotnet test dotnet/ -c Release
cd node   && npm ci && npm run lint && npm run typecheck && npm test
cd python && pip install -e .[dev] && ruff check . && mypy src && pytest
cd rust   && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test
cd go     && go vet ./... && golangci-lint run ./... && go test ./...
cd java   && mvn -B verify
cd ruby   && bundle install && bundle exec standardrb && bundle exec rspec
cd php    && composer install && vendor/bin/phpstan analyse && XDEBUG_MODE=coverage vendor/bin/phpunit
```

CI runs all of the above plus coverage on every PR. **Linters and formatters are enforced** — run them
before pushing.

## Conventions (please keep these)

- **Money is never a float** — `decimal` (.NET), `Decimal` (Python), `BigDecimal` (Java), `string`
  (Node/Go/PHP), exact types (Rust/Ruby).
- **No secrets in logs or error messages** — never log the private key, shared secret, plaintext, or API key.
- **The wire format is fixed** — if you change the envelope scheme you must regenerate `fixtures/`
  (`node tools/generate-fixtures.mjs`) so all eight SDKs stay in lockstep.
- Keep each SDK's public surface consistent: `getAccounts`, `getTransactions`, `getConnections`, `sync`,
  `syncAll`.

## Pull requests

1. Branch, make your change, run the relevant gates locally.
2. Open a PR — the **Changed packages** bot comments which SDKs you touched.
3. **Do not bump versions in your PR.** A maintainer cuts releases via **Actions → Release** (see
   [`RELEASING.md`](RELEASING.md)).

The PR template includes a short security checklist — please fill it in.

## Reporting issues

Bugs and features → GitHub Issues (templates provided). Security vulnerabilities → **do not** open a public
issue; see [`SECURITY.md`](SECURITY.md).
