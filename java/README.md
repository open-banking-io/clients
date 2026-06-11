<p align="center">
  <a href="https://open-banking.io">
    <img src="https://raw.githubusercontent.com/open-banking-io/clients/main/.github/logo.png" alt="open-banking.io" height="56">
  </a>
</p>

# open-banking.io (Java)

Server-to-server client for [open-banking.io](https://open-banking.io): API-key auth +
client-side decryption of the zero-knowledge data envelopes with your exported private key.

```xml
<dependency>
  <groupId>io.open-banking</groupId>
  <artifactId>open-banking-io-client</artifactId>
  <version>0.2.1</version>
</dependency>
```

```java
import io.openbanking.*;

// Load the credentials .json you exported from the app (API key + private key).
var client = OpenBankingClient.fromCredentials("credentials.json");
for (Account a : client.getAccounts()) {
    a.balances().stream()
        .filter(b -> b.type().equals("ITBD"))
        .findFirst()
        .ifPresent(b -> System.out.println(a.iban() + " " + b.amount() + " " + a.currency()));

    TransactionPage page = client.getTransactions(a.id(), TransactionQuery.none().withLimit(50));
    for (Transaction t : page.items())
        System.out.println("  " + t.bookingDate() + "  " + t.creditorName() + "  " + t.amount() + " " + t.currency());

    // Trigger an online sync (decrypts the account uid locally and posts it):
    client.sync(a.id());
}
```

Requires **Java 17+**. The client is built on the JDK's `java.net.http.HttpClient` and `javax.crypto`
(JCA); the only third-party dependency is Jackson for JSON. It exposes the same surface as the other
SDKs: `getAccounts`, `getTransactions`, `getConnections`, `sync`, `syncAll`. `sync` decrypts the
account's session uid locally and posts it, so the service can refresh from the bank without ever
holding it in plaintext.

Monetary amounts are returned as `BigDecimal`; other optional fields are `null` when absent. The
public models are Java `record`s.

The default client uses a 10s connect timeout and a 30s per-request timeout, and sends a
`User-Agent: open-banking-io/java/<version>` header on every request; an injected `HttpClient` is
used as-is.

## Encryption

Envelopes use **ECDH P-256 → HKDF-SHA256 → AES-256-GCM**, implemented with `javax.crypto` (JCA).
Decryption requires the private key from your credentials bundle and happens entirely in-process. The
library is verified against the shared `fixtures/` so it decrypts identically to the other SDKs. Full
wire format and the other language clients:
[repo README](https://github.com/open-banking-io/clients) ·
[`THREAT_MODEL.md`](https://github.com/open-banking-io/clients/blob/main/THREAT_MODEL.md).

## Development

```bash
mvn -B verify   # crypto round-trip + a mock-server integration suite
```

MIT licensed.
