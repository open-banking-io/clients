# Changelog

All notable changes to the open-banking.io client SDKs are documented here.

This is a monorepo of independently versioned packages, each released under its own
`<package>/vX.Y.Z` git tag and following [Semantic Versioning](https://semver.org/):

| Package | Tag prefix | Registry |
| ------- | ---------- | -------- |
| .NET    | `dotnet/v` | NuGet |
| Node    | `node/v`   | npm |
| Python  | `python/v` | PyPI |
| Rust    | `rust/v`   | crates.io |
| Go      | `go/v`     | Go modules |
| Java    | `java/v`   | Maven Central |
| Ruby    | `ruby/v`   | RubyGems |
| PHP     | `php/v`    | Packagist |
| CLI     | `cli/v`    | GitHub Releases + Homebrew |
| n8n     | `n8n/v`    | npm |

## Per-release notes

Detailed, auto-generated notes for every release live on the
[GitHub Releases page](https://github.com/open-banking-io/clients/releases), keyed by the
`<package>/vX.Y.Z` tag. The release process is documented in [`RELEASING.md`](RELEASING.md).

## Notable cross-cutting changes

### 2026-06

- Security hardening for the OpenSSF Scorecard: added CodeQL static analysis, native Go
  fuzzing of the envelope parser, cosign signing of CLI release artifacts, least-privilege
  workflow tokens, and remediation of all known dependency vulnerabilities.
