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
openbanking accounts                    # list accounts with balances
openbanking transactions <account-id>   # an account's statement (--from --to --limit)
openbanking sync --all                  # pull fresh transactions
openbanking banks                       # banks available to connect
openbanking connect <bank-name>         # connect a bank via the consent flow
openbanking version
```

Credentials live in `$OPENBANKING_CONFIG` or
`<XDG_CONFIG_HOME|~/.config>/open-banking/credentials.json` (written `0600`).

## How it works

`login` runs a localhost loopback + PKCE flow against the API. In the browser you
sign in and enter your passphrase; the web app unlocks your P-256 encryption key
and hands the full credentials straight to the CLI's loopback. **The private key
travels browser → localhost only — it never touches the server, not even encrypted**
(the server is zero-knowledge and only ever holds your public key). `key import`
remains as a manual fallback for headless machines without a browser. Money amounts
are kept as exact decimal strings; debits render negative at display time.
