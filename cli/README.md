# obank

The **open-banking.io** command line. Authenticate once, then read, sync, and
connect your bank data locally — the zero-knowledge envelopes are decrypted
in-process with your own key, never on a server.

## Install

### Homebrew (macOS / Linux)

```sh
brew install open-banking-io/tap/obank
```

### Binaries (Windows / macOS / Linux)

Download the archive for your platform from the
[latest release](https://github.com/open-banking-io/clients/releases?q=obank),
unpack it, and put `obank` on your `PATH`. Windows ships a `.zip`; macOS/Linux a
`.tar.gz`. Checksums are published alongside as `checksums.txt`.

### From source

```sh
go install github.com/open-banking-io/clients/cli@latest
```

## Usage

```sh
obank login                       # sign in via the browser, saves an API key
obank key import ./credentials.json   # import your encryption key (exported from the app)
obank accounts                    # list accounts with balances
obank transactions <account-id>   # an account's statement (--from --to --limit)
obank sync --all                  # pull fresh transactions
obank banks                       # banks available to connect
obank connect <bank-name>         # connect a bank via the consent flow
obank version
```

Credentials live in `$OPENBANKING_CONFIG` or
`<XDG_CONFIG_HOME|~/.config>/open-banking/credentials.json` (written `0600`).

## How it works

`login` runs a localhost loopback + PKCE flow against the API to mint an API key
without a browser cookie. `key import` adds your P-256 encryption key (exported
from the web app) so accounts and transactions can be decrypted locally. Money
amounts are kept as exact decimal strings; debits render negative at display time.
