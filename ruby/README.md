# open-banking-io (Ruby)

Server-to-server client for [open-banking.io](https://open-banking.io). It authenticates with your
**API key** and decrypts the **zero-knowledge** data envelopes locally with your exported **private
key** — the service only ever returns ciphertext it cannot read.

```bash
gem install open-banking-io
```

```ruby
require "open_banking_io"

# Load the credentials .json you exported from the app (API key + private key).
client = OpenBankingIO::Client.from_credentials("credentials.json")

client.get_accounts.each do |account|
  booked = account.balances.find { |b| b.type == "ITBD" }
  label = account.display_name || account.owner_name
  puts "#{label} #{account.iban}: #{booked&.amount} #{account.currency}"

  page = client.get_transactions(account.id, limit: 50)
  page.items.each do |t|
    puts "  #{t.booking_date}  #{t.creditor_name || t.debtor_name}  #{t.amount} #{t.currency}"
  end

  # Trigger an online sync (decrypts the account uid locally and posts it):
  client.sync(account.id)
end
```

Or construct it explicitly:

```ruby
client = OpenBankingIO::Client.new(
  api_base_url: api_base_url,
  api_key: api_key,
  private_key_pkcs8: private_key_pkcs8
)
```

## API

- `get_accounts` → `Array<Account>` — decrypts each account's envelope, display name and balances.
- `get_transactions(account_id, from: nil, to: nil, limit: nil, offset: nil)` → `TransactionPage`
- `get_connections` → `Array<Connection>`
- `sync(account_id)` → `SyncResult` — decrypts the account uid locally and posts it.
- `sync_all` → `SyncAllResult` — syncs every account that has an active session.

Amounts are exposed as `BigDecimal`. Models are immutable keyword-initialised `Struct`s.

## Encryption

Envelopes use **ECDH P-256 → HKDF-SHA256 → AES-256-GCM**, implemented entirely with Ruby's OpenSSL
standard library. Decryption requires the private key from your credentials bundle and happens fully
in-process. See the [repo README](https://github.com/open-banking-io/clients) for the full scheme and
the other language clients (.NET, Node, Python, Rust, Go, Java).

## Development

```bash
bundle install
bundle exec rspec
```

MIT licensed.
