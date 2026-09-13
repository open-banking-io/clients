# Changelog

All notable changes to the open-banking.io client SDKs are documented here.

This is a monorepo of independently versioned packages, each released under its own
`<package>/vX.Y.Z` git tag and following [Semantic Versioning](https://semver.org/):

| Package   | Tag prefix    | Registry                   |
| --------- | ------------- | -------------------------- |
| .NET      | `dotnet/v`    | NuGet                      |
| Node      | `node/v`      | npm                        |
| Python    | `python/v`    | PyPI                       |
| Rust      | `rust/v`      | crates.io                  |
| Go        | `go/v`        | Go modules                 |
| Java      | `java/v`      | Maven Central              |
| Ruby      | `ruby/v`      | RubyGems                   |
| PHP       | `php/v`       | Packagist                  |
| CLI       | `cli/v`       | GitHub Releases + Homebrew |
| n8n       | `n8n/v`       | npm                        |
| Beancount | `beancount/v` | PyPI                       |

## Per-release notes

Detailed, auto-generated notes for every release live on the
[GitHub Releases page](https://github.com/open-banking-io/clients/releases), keyed by the
`<package>/vX.Y.Z` tag. The release process is documented in [`RELEASING.md`](RELEASING.md).

## Notable cross-cutting changes

### 2026-09

- CLI (on Go 0.5.0): `connections` gains an `ACCESS` column (`live`, `live, Nd left`, `ended`) and
  `isLive`/`accountIds` in JSON. **Breaking for scripts:** `sync --all` now exits 1 when any account
  could not be refreshed (a lapsed consent included), printing each with its reason, bank code and
  what to do. Outside a terminal, or with `-o json`/`-o csv`, both `sync` and `sync --all` print a
  JSON document instead of the prose summary, as the listing commands already do; for `--all` its
  `failures` carry the same list.
- Node 1.3.0, Go 0.5.0: consent renewal and sync failures. `Connection` gains `isLive` and
  `accountIds`; `syncAll` returns `failures` (`reconnect_needed`, `uid_outdated`,
  `psu_present_required`, …) instead of dropping them, and a refused single sync throws a typed
  `SyncError` with `reason`, `bankErrorCode` and `retryAfterSeconds`. Both re-read the account and
  retry once on `uid_outdated`. A sync can forward the present user's request as `X-Psu-*` headers
  (`{ psu }` / `SyncOptions{Psu}`). Node only: `buildAuthorizeUrl({ renewConnection })` and
  `closeReplacedConsents`, which closes the consents a renewal replaced in the revoke of the
  previous key. `syncAll` also reports every account whose consent needs renewing as
  `reconnect_needed` (it has no uid to send, and used to vanish from the result). New fixture
  `api/sync-all-failures.json`; `api/connections.json` gains `isLive` and `accountIds`. Go: the new
  slice fields make `Connection` and `SyncAllResult` no longer comparable with `==`.
- Node 1.2.0, Go 0.4.1, CLI: every partner now holds its own decryption key on open-banking.io,
  and the relay carries an empty `privateKey` for its users. `parseRelay` takes
  `expectPrivateKey: "optional"` (the default stays `"required"`), `RelayError.reason` names the
  two partner-side refusals (`partner_key_missing`, `partner_suspended`), and
  `generateRecipientKeyPair`, `recipientKeyFingerprint`, `recipientPublicKey` and
  `answerRecipientKeyChallenge` cover the key itself. Go exports `DecryptEnvelope`/`DecryptTo`;
  the CLI gains `openbanking partner key generate|fingerprint|answer`. A new fixture,
  `recipient-key-challenge.json`, pins the possession challenge for every SDK. (Go `v0.4.0` was
  tagged on the commit before these exports and is already in the module proxy, so it is
  identical to 0.3.0; use 0.4.1.)

### 2026-08

- Node 1.1.0: a `connect` module for Partner Connect — PKCE and state helpers, `buildAuthorizeUrl`,
  `parseRelay` (constant-time `state`/`iss` checks, RFC error surfacing), `exchangeCode`,
  `revokeToken`, `userinfo`, `discover` and `OpenBankingClient.fromTokenResponse`. The flow is plain
  OAuth 2.0 with PKCE and `form_post`; other SDKs can use any client library that supports those, and
  the CLI's Go `pkce.go`/relay parser is the template for a Go port.

### 2026-06

- Security hardening for the OpenSSF Scorecard: added CodeQL static analysis, native Go
  fuzzing of the envelope parser, cosign signing of CLI release artifacts, least-privilege
  workflow tokens, and remediation of all known dependency vulnerabilities.
