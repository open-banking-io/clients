# Releasing

This monorepo ships **eight independent client SDKs** from one repo. Each is released on its own, and
**only the package you tag gets published**.

## The tag scheme (the contract)

Every release is a **package-prefixed git tag**:

```
<dir>/vX.Y.Z
```

where `<dir>` ∈ `dotnet`, `node`, `python`, `rust`, `java`, `go`, `ruby`, `php`, `beancount`, `erpnext`,
`zapier`, `cli`, and `X.Y.Z` is a [SemVer](https://semver.org) version (the literal `v` is part of the
prefix, e.g. `node/v0.2.0`).

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
| `beancount` | `beancount/pyproject.toml` (`[project].version`) | — |
| `erpnext` | `erpnext/pyproject.toml` (`[project].version`) | `erpnext/erpnext_open_banking/__init__.py` (`__version__`) |
| `zapier` | `zapier/package.json` (`version`) | `zapier/package-lock.json` |

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

### CLI specifics (`obank`)

The `cli` package is the `obank` binary, not a library — it has no manifest either
(its version comes from the tag, baked in via `-ldflags`). Tagging `cli/vX.Y.Z`
triggers `publish-cli.yml`, which cross-compiles binaries for linux/macOS/windows
(amd64 + arm64), publishes a GitHub Release with archives + `checksums.txt`, and
updates the **Homebrew tap** (`open-banking-io/homebrew-tap`). It's a plain
`go build` matrix (not GoReleaser, whose monorepo/tag-prefix support is Pro-only)
so it works cleanly with the `cli/`-prefixed tags. On PRs touching the CLI the same
job builds all targets as a snapshot to validate, without publishing.

Because the CLI ships as a binary built in module mode (`GOWORK=off`), it depends
on a **published** `go/` SDK version. Release the SDK first if the CLI needs new
SDK code: cut `go/vX.Y.Z`, bump `cli/go.mod`'s `require .../clients/go` to it, then
cut `cli/vX.Y.Z`.

Prerequisites for the Homebrew step: the `open-banking-io/homebrew-tap` repo must
exist, and a `HOMEBREW_TAP_TOKEN` secret (a PAT with write access to that tap repo)
must be set on this repo — the default `GITHUB_TOKEN` can't push to another repo.

### ERPNext app specifics (`erpnext_open_banking`)

The `erpnext` package is a **Frappe/ERPNext app**, distributed by git (there is no
registry — `bench get-app` clones a repo). Tagging `erpnext/vX.Y.Z` triggers
`publish-erpnext.yml`, which runs the pytest suite, verifies `tag == pyproject ==
__init__.__version__`, then **subtree-splits** `erpnext/` and force-pushes it (plus
the `vX.Y.Z` tag) to the **`open-banking-io/erpnext` mirror**, where the app sits at
the repo root so users can:

```bash
bench get-app https://github.com/open-banking-io/erpnext --branch v0.1.0
bench --site <site> install-app erpnext_open_banking
```

Same subtree-split model as `n8n`, `php`, and `zapier`. Requires the
`MIRROR_PUSH_TOKEN` secret (shared with the other mirrors) and the mirror repo to
exist. Anything committed only to the mirror is wiped by the next release's
force-push — the monorepo `erpnext/` is the single source of truth.

## The version-consistency gate

Each `publish-<pkg>.yml` compares `tag == manifest` and **fails** on mismatch (skipped for `go`/`php`,
which are tag-only). So the manifest must be on `main` at the released version before you tag.

## What changed in a PR?

Every PR gets a **Changed packages** comment listing which client directories changed and which
package(s) to release after merge. A change under `fixtures/**` (shared vectors) affects **all** clients.
