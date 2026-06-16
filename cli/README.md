<p align="center">
  <a href="https://open-banking.io">
    <img src="https://raw.githubusercontent.com/open-banking-io/clients/main/.github/logo.png" alt="open-banking.io" height="56">
  </a>
</p>

# open-banking.io CLI (`openbanking`)

The **open-banking.io** command line. Authenticate once, then read, sync, and
connect your bank data locally — the zero-knowledge envelopes are decrypted
in-process with your own key, never on a server.

## Install

### Homebrew (macOS / Linux)

```sh
brew install open-banking-io/tap/openbanking
```

### Binaries (Windows / macOS / Linux)

Download the archive for your platform from the
[latest release](https://github.com/open-banking-io/clients/releases?q=openbanking),
unpack it, and put `openbanking` on your `PATH`. Windows ships a `.zip`; macOS/Linux a
`.tar.gz`. Checksums are published alongside as `checksums.txt`.

### From source

```sh
go install github.com/open-banking-io/clients/cli@latest
```

## Usage

```sh
openbanking login                       # sign in + unlock in the browser — sets up everything
openbanking key import ./credentials.json   # (fallback) import a key file on a headless machine
openbanking accounts                    # list accounts with balances        (alias: acc, ls)
openbanking use                         # pick a current account (arrow keys) — or: use <account-id>
openbanking transactions                # the current account's statement     (alias: tx)
openbanking transactions <account-id>   # a specific account (--from --to --limit --offset)
openbanking sync                        # pull fresh transactions for the current account
openbanking sync --all                  # …or for every connected account
openbanking connections                 # list bank connections               (alias: conn)
openbanking banks                       # list available banks (connect them in the web app)
openbanking                             # no args, in a terminal → interactive menu
openbanking version
```

Run `openbanking` with no command in a terminal for an interactive menu, or
`openbanking help` for the full command list.

### Current account

`use` remembers a default account so `transactions` and `sync` need no id. Run
`openbanking use` to pick one interactively, or `openbanking use <account-id>` to set it
directly. An explicit id on `transactions`/`sync` always overrides it. The choice is stored
in `state.json` beside your credentials (it is CLI-local — never sent anywhere).

### Output formats

Every listing command takes a global `-o`/`--output`:

```sh
openbanking accounts -o json | jq         # json (also the default when output is piped)
openbanking transactions -o csv > tx.csv  # csv for spreadsheets
openbanking accounts -o table             # the pretty, colored table (default in a terminal)
```

Color is on for terminals and off when piped; disable it with `--no-color` or the
[`NO_COLOR`](https://no-color.org) environment variable.

### Shell completion

```sh
source <(openbanking completion zsh)    # also: bash, fish
```

> Connecting a new bank is done in the web app (it needs an interactive consent
> redirect), so the CLI has no `connect` command — it reads and syncs existing connections.

Credentials live in `$OPENBANKING_CONFIG` or
`<XDG_CONFIG_HOME|~/.config>/open-banking/credentials.json` (written `0600`).

Set `$OPENBANKING_API_BASE_URL` to point every command at a different environment
(e.g. staging or local) without re-running `login` — it overrides the saved
bundle's API base URL when non-empty.

## How it works

`login` runs a localhost loopback + PKCE flow against the API. In the browser you
sign in and enter your passphrase; the web app unlocks your P-256 encryption key
and hands the full credentials straight to the CLI's loopback. **The private key
travels browser → localhost only — it never touches the server, not even encrypted**
(the server is zero-knowledge and only ever holds your public key). `key import`
remains as a manual fallback for headless machines without a browser. Money amounts
are kept as exact decimal strings; debits render negative at display time.
