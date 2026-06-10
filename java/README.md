# open-banking.io (Java)

Server-to-server client for [open-banking.io](https://open-banking.io): API-key auth +
client-side decryption of the zero-knowledge data envelopes with your exported private key.

```xml
<dependency>
  <groupId>io.openbanking</groupId>
  <artifactId>open-banking-io-client</artifactId>
  <version>0.1.0</version>
</dependency>
```

```java
import io.openbanking.*;

var client = OpenBankingClient.fromCredentials("credentials.json");
for (Account a : client.getAccounts()) {
    a.balances().stream()
        .filter(b -> b.type().equals("ITBD"))
        .findFirst()
        .ifPresent(b -> System.out.println(a.iban() + " " + b.amount() + " " + a.currency()));
}
```

Requires **Java 17+**. The client is built on the JDK's `java.net.http.HttpClient` and `javax.crypto`
(JCA); the only third-party dependency is Jackson for JSON. It exposes the same surface as the other
SDKs: `getAccounts`, `getTransactions`, `getConnections`, `sync`, `syncAll`. `sync` decrypts the
account's session uid locally and posts it, so the service can refresh from the bank without ever
holding it in plaintext.

Monetary amounts are returned as `BigDecimal`; other optional fields are `null` when absent. The
public models are Java `record`s.

## Encryption scheme

Each sensitive value is an envelope: `version(1) | ephemeralPublicKey(65) | nonce(12) | tag(16) |
ciphertext`, produced with ephemeral ECDH on P-256 → HKDF-SHA256 (salt = 32 zero bytes, info =
`bank.core.ci/zk/v1`) → AES-256-GCM. Only your private key can open it. The library is verified
against the shared `fixtures/` so it decrypts identically to the other SDKs.

## Development

```bash
mvn -B verify   # crypto round-trip + a mock-server integration suite
```

MIT licensed.
