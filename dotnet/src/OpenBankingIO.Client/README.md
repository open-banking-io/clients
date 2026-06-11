<p align="center">
  <a href="https://open-banking.io">
    <img src="https://raw.githubusercontent.com/open-banking-io/clients/main/.github/logo.png" alt="open-banking.io" height="56">
  </a>
</p>

# OpenBankingIO.Client (.NET)

Server-to-server client for [open-banking.io](https://open-banking.io). It authenticates with your
**API key** and decrypts the **zero-knowledge** data envelopes locally with your exported **private
key** — the service only ever returns ciphertext it cannot read.

```bash
dotnet add package OpenBankingIO.Client
```

```csharp
using OpenBankingIO.Client;

// Load the credentials .json you exported from the app (API key + private key).
using var client = OpenBankingClient.FromCredentials("credentials.json");

foreach (var account in await client.GetAccountsAsync())
{
    var booked = account.Balances.FirstOrDefault(b => b.Type == "ITBD");
    Console.WriteLine($"{account.DisplayName ?? account.OwnerName} {account.Iban}: {booked?.Amount} {account.Currency}");

    var page = await client.GetTransactionsAsync(account.Id, limit: 50);
    foreach (var t in page.Items)
        Console.WriteLine($"  {t.BookingDate:yyyy-MM-dd}  {t.CreditorName ?? t.DebtorName}  {t.Amount} {t.Currency}");
}

// Trigger an online sync (decrypts the account uid locally and posts it):
await client.SyncAsync(accountId);
```

When the client creates its own `HttpClient` it applies a 30s request timeout and sends `User-Agent: open-banking-io/dotnet/<version>`; a caller-supplied `HttpClient` is left untouched.

Or construct it explicitly:

```csharp
using var client = new OpenBankingClient(apiBaseUrl, apiKey, privateKeyPkcs8Base64);
```

## Encryption

Envelopes use **ECDH P-256 → HKDF-SHA256 → AES-256-GCM**. Decryption requires the private key from
your credentials bundle and happens entirely in-process; amounts are exposed as `decimal`. Full wire
format and the other language clients:
[repo README](https://github.com/open-banking-io/clients) ·
[`THREAT_MODEL.md`](https://github.com/open-banking-io/clients/blob/main/THREAT_MODEL.md).

MIT licensed.
