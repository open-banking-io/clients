# Releasing

This monorepo ships **seven independent client SDKs** from one repo. Each is
released on its own, and **only the package you tag gets published**.

## The tag scheme (the contract)

Every release is a **package-prefixed git tag**:

```
<dir>/vX.Y.Z
```

where `<dir>` is one of: `dotnet`, `node`, `python`, `rust`, `java`, `go`, `ruby`,
and `X.Y.Z` is a [SemVer](https://semver.org) version (no leading `v` in the
version part — the literal `v` is part of the prefix, e.g. `node/v0.2.0`).

Each `publish-<dir>.yml` workflow triggers **only** on its own `<dir>/v*` tags, so
tagging `node/v0.2.0` publishes the Node package and nothing else.

## Where each package's version lives

For all packages except Go, the version is kept in a manifest and **must match the
tag** — each `publish-<dir>.yml` has a gate that fails the publish if `tag != manifest`.

| Package | Manifest (source of truth) | Also updated |
|---|---|---|
| `dotnet` | `dotnet/src/OpenBankingIO.Client/OpenBankingIO.Client.csproj` (`<Version>`) | — |
| `node`   | `node/package.json` (`version`) | `node/package-lock.json` |
| `python` | `python/pyproject.toml` (`[project].version`) | — |
| `rust`   | `rust/Cargo.toml` (`[package].version`) | `rust/Cargo.lock` |
| `java`   | `java/pom.xml` (project `<version>`) | — |
| `ruby`   | `ruby/lib/open_banking_io/version.rb` (`VERSION`) / gemspec | — |
| `go`     | **none** — the version *is* the tag | — |

## Cutting a release (the normal path)

Use the **Release** workflow — it does the manifest bump, the commit, and the tag
for you.

1. Go to **Actions → Release → Run workflow**.
2. Pick the **package** (e.g. `node`) and enter the **version** (e.g. `0.2.0`).
3. Run it. The workflow will:
   - validate the version is semver and **refuse if the tag already exists**;
   - for every package **except `go`**: run `node tools/bump-version.mjs <pkg> <version>`
     to update the manifest (and lockfiles), then commit
     `release(<pkg>): v<version>` to `main` as the release bot;
   - create and push the tag `<pkg>/v<version>`.
4. Pushing the tag triggers `publish-<pkg>.yml`, which verifies the tag matches the
   manifest and publishes to the registry (NuGet / npm / PyPI / crates.io / Maven
   Central / pkg.go.dev / RubyGems).

### Go specifics

Go has no manifest — the module version is the tag itself. The Release workflow
**skips the bump/commit** for `go` and just pushes `go/vX.Y.Z`. Consumers then get
that version via `go get github.com/open-banking-io/clients/go@vX.Y.Z`.

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

## The version-consistency gate

Don't hand-edit a manifest and tag separately unless you keep them identical — the
`publish-<pkg>.yml` gate compares `tag == manifest` and **fails** on mismatch. The
Release workflow keeps them in lockstep, so prefer it.

## Manual bump (advanced / local)

If you need to bump a manifest without releasing (e.g. in a PR), run the same
script the workflow uses:

```bash
node tools/bump-version.mjs <package> <version>   # e.g. node 0.2.0
```

Runs on Node 20+, no dependencies. It validates semver, edits the correct
manifest(s), and throws for `go` (which has no manifest).

## What changed in a PR?

Every PR gets a **Changed packages** comment listing which client directories
changed and which package(s) to release after merge. A change under `fixtures/**`
(shared test vectors) is reported as affecting **all** clients. If only shared
infra changed, the comment says there's nothing to release.
