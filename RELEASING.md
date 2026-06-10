# Releasing

This monorepo ships **eight independent client SDKs** from one repo. Each is released on its own, and
**only the package you tag gets published**.

## The tag scheme (the contract)

Every release is a **package-prefixed git tag**:

```
<dir>/vX.Y.Z
```

where `<dir>` ∈ `dotnet`, `node`, `python`, `rust`, `java`, `go`, `ruby`, `php`, and `X.Y.Z` is a
[SemVer](https://semver.org) version (the literal `v` is part of the prefix, e.g. `node/v0.2.0`).

Each `publish-<dir>.yml` triggers **only** on its own `<dir>/v*` tags, so tagging `node/v0.2.0` publishes
the Node package and nothing else.

## Where each package's version lives

| Package | Source of truth | Also updated |
|---|---|---|
| `dotnet` | `dotnet/src/OpenBankingIO.Client/OpenBankingIO.Client.csproj` (`<Version>`) | — |
| `node`   | `node/package.json` (`version`) | `node/package-lock.json` |
| `python` | `python/pyproject.toml` (`[project].version`) | — |
| `rust`   | `rust/Cargo.toml` (`[package].version`) | `rust/Cargo.lock` |
| `java`   | `java/pom.xml` (project `<version>`) | — |
| `ruby`   | `ruby/lib/open_banking_io/version.rb` (`VERSION`) | gemspec |
| `go`     | **none** — version *is* the tag; `Version` const in `go/client.go` is cosmetic (User-Agent) | — |
| `php`    | **none** — Packagist derives version from the tag; `Client::VERSION` const is cosmetic | — |

## Cutting a release (two steps — `main` is protected)

`main` is a protected branch (PRs + status checks required), so releases do **not** push to `main`
directly. A release is two steps:

**1. Bump the version in a PR.** For `dotnet/node/python/rust/java/ruby`, bump the manifest:

```bash
node tools/bump-version.mjs <package> <version>   # e.g. node 0.2.0  (updates manifest + lockfiles)
```

For `go`/`php`, update the cosmetic `Version`/`VERSION` const to match. Open the PR, let CI pass, merge.

**2. Tag via the Release workflow.** Go to **Actions → Release → Run workflow**, pick the **package** and
**version**. It will:
- validate semver and **refuse if the tag already exists**;
- for non-`go`/`php`: verify the manifest on `main` is **already** at that version (fails with a clear
  message if you forgot step 1);
- create and push the tag `<pkg>/v<version>` and a **GitHub Release** with auto-generated notes.

Pushing the tag triggers `publish-<pkg>.yml`, which re-checks `tag == manifest` and publishes to the
registry (NuGet / npm / PyPI / crates.io / Maven Central / pkg.go.dev / RubyGems / Packagist).

> You can also just push the tag by hand (`git tag <pkg>/v<version> && git push origin <pkg>/v<version>`)
> once the version is on `main` — tags aren't branch-protected. The Release workflow just adds the guards
> and release notes.

## The version-consistency gate

Each `publish-<pkg>.yml` compares `tag == manifest` and **fails** on mismatch (skipped for `go`/`php`,
which are tag-only). So the manifest must be on `main` at the released version before you tag.

## What changed in a PR?

Every PR gets a **Changed packages** comment listing which client directories changed and which
package(s) to release after merge. A change under `fixtures/**` (shared vectors) affects **all** clients.
