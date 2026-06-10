package io.openbanking;

import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.JavaType;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.io.IOException;
import java.math.BigDecimal;
import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.interfaces.ECPrivateKey;
import java.time.Duration;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * Server-to-server client for open-banking.io. Authenticates with an API key ({@code X-Api-Key})
 * and decrypts the zero-knowledge data envelopes locally with the exported private key — the
 * service only ever returns ciphertext it cannot read.
 */
public final class OpenBankingClient {

  /** Connect timeout for the default HTTP client. */
  private static final Duration CONNECT_TIMEOUT = Duration.ofSeconds(10);

  /** Per-request timeout applied to every request this client sends. */
  private static final Duration REQUEST_TIMEOUT = Duration.ofSeconds(30);

  /** User-Agent sent on every request; {@code <version>} comes from the JAR manifest. */
  private static final String USER_AGENT = "open-banking-io/java/" + version();

  private final String baseUrl;
  private final String apiKey;
  private final ECPrivateKey privateKey;
  private final HttpClient http;
  private final ObjectMapper mapper =
      new ObjectMapper().configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false);

  /**
   * @param apiBaseUrl e.g. {@code https://open-banking.io}
   * @param apiKey the API key from your credentials bundle
   * @param privateKeyPkcs8Base64 the base64 PKCS#8 encryption private key from your bundle
   * @param httpClient an injected client (e.g. for testing), or {@code null} for a default one
   */
  public OpenBankingClient(
      String apiBaseUrl, String apiKey, String privateKeyPkcs8Base64, HttpClient httpClient) {
    if (isBlank(apiBaseUrl)) {
      throw new OpenBankingException("apiBaseUrl is required");
    }
    if (isBlank(apiKey)) {
      throw new OpenBankingException("apiKey is required");
    }
    if (isBlank(privateKeyPkcs8Base64)) {
      throw new OpenBankingException("privateKeyPkcs8 is required");
    }
    this.baseUrl = apiBaseUrl.replaceAll("/+$", "");
    this.apiKey = apiKey;
    this.privateKey = Envelope.loadPrivateKey(privateKeyPkcs8Base64);
    this.http =
        httpClient != null
            ? httpClient
            : HttpClient.newBuilder().connectTimeout(CONNECT_TIMEOUT).build();
  }

  /**
   * The implementation version baked into the JAR manifest by Maven, or {@code "dev"} when running
   * from a build tree without a manifest (e.g. tests).
   */
  private static String version() {
    String v = OpenBankingClient.class.getPackage().getImplementationVersion();
    return (v == null || v.isBlank()) ? "dev" : v;
  }

  /** Builds a client from a credentials-bundle JSON string or a path to a bundle file. */
  public static OpenBankingClient fromCredentials(String jsonOrPath) {
    return fromCredentials(jsonOrPath, null);
  }

  public static OpenBankingClient fromCredentials(String jsonOrPath, HttpClient httpClient) {
    String raw = jsonOrPath;
    if (!jsonOrPath.stripLeading().startsWith("{")) {
      try {
        raw = Files.readString(Path.of(jsonOrPath));
      } catch (IOException e) {
        throw new OpenBankingException("could not read credentials file", e);
      }
    }
    CredentialsBundle bundle;
    try {
      bundle =
          new ObjectMapper()
              .configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false)
              .readValue(raw, CredentialsBundle.class);
    } catch (IOException e) {
      throw new OpenBankingException("could not parse the credentials bundle", e);
    }
    return fromBundle(bundle, httpClient);
  }

  public static OpenBankingClient fromBundle(CredentialsBundle bundle, HttpClient httpClient) {
    if (bundle == null || isBlank(bundle.apiKey())) {
      throw new OpenBankingException("the credentials bundle has no apiKey");
    }
    if (bundle.encryptionKey() == null || isBlank(bundle.encryptionKey().privateKey())) {
      throw new OpenBankingException("the credentials bundle has no encryption private key");
    }
    return new OpenBankingClient(
        bundle.apiBaseUrl(), bundle.apiKey(), bundle.encryptionKey().privateKey(), httpClient);
  }

  /** Lists the user's accounts with all sensitive fields decrypted. */
  public List<Account> getAccounts() {
    List<Account> accounts = new ArrayList<>();
    for (AccountWire w : getAccountWires()) {
      accounts.add(mapAccount(w));
    }
    return accounts;
  }

  /** Returns a page of an account's statement, newest first, with decrypted fields. */
  public TransactionPage getTransactions(String accountId, TransactionQuery query) {
    if (query == null) {
      query = TransactionQuery.none();
    }
    StringBuilder qs = new StringBuilder();
    appendParam(qs, "from", query.from());
    appendParam(qs, "to", query.to());
    appendParam(qs, "limit", query.limit() == null ? null : query.limit().toString());
    appendParam(qs, "offset", query.offset() == null ? null : query.offset().toString());

    String path = "/api/accounts/" + encodePathSegment(accountId) + "/transactions" + qs;
    TransactionPageWire page = getJson(path, TransactionPageWire.class);

    List<Transaction> items = new ArrayList<>();
    List<TransactionWire> wireItems = page.items() == null ? List.of() : page.items();
    for (TransactionWire t : wireItems) {
      items.add(mapTransaction(t));
    }
    return new TransactionPage(items, page.total());
  }

  /** Lists the user's bank connections. */
  public List<Connection> getConnections() {
    List<ConnectionWire> wires = getJsonList("/api/connections", ConnectionWire.class);
    List<Connection> conns = new ArrayList<>();
    for (ConnectionWire c : wires) {
      conns.add(
          new Connection(
              c.sessionId(),
              c.aspspName(),
              c.aspspCountry(),
              c.validUntil(),
              c.status(),
              c.accountCount(),
              c.lastSyncedAt(),
              c.psuType()));
    }
    return conns;
  }

  /**
   * Triggers an online sync of one account: decrypts that account's Enable Banking uid and posts
   * it, so the service can fetch fresh data without ever holding the uid in plaintext.
   */
  public SyncResult sync(String accountId) {
    AccountWire account = null;
    for (AccountWire a : getAccountWires()) {
      if (a.id().equals(accountId)) {
        account = a;
        break;
      }
    }
    if (account == null) {
      throw new OpenBankingException("account " + accountId + " not found");
    }
    String uid = decryptUid(account);
    if (uid == null) {
      throw new OpenBankingException(
          "account has no active session (reconnect required) — cannot sync");
    }

    SyncResultWire result =
        postJson(
            "/api/accounts/" + encodePathSegment(accountId) + "/sync",
            Map.of("uid", uid),
            SyncResultWire.class);
    return new SyncResult(result.newTransactions(), result.totalFetched());
  }

  /** Triggers an online sync of every account that has an active session. */
  public SyncAllResult syncAll() {
    List<Map<String, String>> items = new ArrayList<>();
    for (AccountWire a : getAccountWires()) {
      String uid = decryptUid(a);
      if (uid != null) {
        Map<String, String> item = new LinkedHashMap<>();
        item.put("accountId", a.id());
        item.put("uid", uid);
        items.add(item);
      }
    }

    SyncAllResultWire result =
        postJson("/api/sync", Map.of("items", items), SyncAllResultWire.class);
    return new SyncAllResult(result.accounts(), result.newTransactions());
  }

  // ---- internals ----

  private List<AccountWire> getAccountWires() {
    return getJsonList("/api/accounts", AccountWire.class);
  }

  private String decryptUid(AccountWire a) {
    UidEnc dec = Envelope.decryptTo(privateKey, a.uidEnc(), mapper, UidEnc.class);
    return dec == null ? null : dec.uid();
  }

  private Account mapAccount(AccountWire a) {
    AccountEnc acc = Envelope.decryptTo(privateKey, a.enc(), mapper, AccountEnc.class);
    DisplayNameEnc name =
        Envelope.decryptTo(privateKey, a.displayNameEnc(), mapper, DisplayNameEnc.class);

    List<Balance> balances = new ArrayList<>();
    List<BalanceWire> wireBalances = a.balances() == null ? List.of() : a.balances();
    for (BalanceWire b : wireBalances) {
      BalanceEnc dec = Envelope.decryptTo(privateKey, b.enc(), mapper, BalanceEnc.class);
      balances.add(
          new Balance(
              b.type(),
              dec == null ? null : dec.name(),
              parseDecimal(dec == null ? null : dec.amount()),
              b.currency(),
              b.referenceDate()));
    }

    return new Account(
        a.id(),
        a.aspspName(),
        a.aspspCountry(),
        a.currency(),
        a.accountType(),
        a.bic(),
        a.needsReconnect(),
        acc == null ? null : acc.iban(),
        acc == null ? null : acc.bban(),
        acc == null ? null : acc.ownerName(),
        acc == null ? null : acc.accountName(),
        acc == null ? null : acc.product(),
        name == null ? null : name.displayName(),
        balances);
  }

  private Transaction mapTransaction(TransactionWire t) {
    TransactionEnc d = Envelope.decryptTo(privateKey, t.enc(), mapper, TransactionEnc.class);
    return new Transaction(
        t.id(),
        t.currency(),
        t.creditDebitIndicator(),
        t.status(),
        t.bookingDate(),
        t.valueDate(),
        t.transactionDate(),
        t.bankTransactionCode(),
        parseDecimal(d == null ? null : d.amount()),
        d == null ? null : d.creditorName(),
        d == null ? null : d.creditorIban(),
        d == null ? null : d.creditorBban(),
        d == null ? null : d.creditorAgentBic(),
        d == null ? null : d.debtorName(),
        d == null ? null : d.debtorIban(),
        d == null ? null : d.debtorBban(),
        d == null ? null : d.debtorAgentBic(),
        d == null ? null : d.remittanceInformation(),
        d == null ? null : d.note(),
        d == null ? null : d.referenceNumber(),
        d == null ? null : d.exchangeRate(),
        d == null ? null : d.merchantCategoryCode(),
        parseDecimalNullable(d == null ? null : d.balanceAfter()),
        d == null ? null : d.balanceAfterCurrency());
  }

  private <T> T getJson(String path, Class<T> type) {
    return readJson(sendGet(path), path, mapper.getTypeFactory().constructType(type));
  }

  private <T> List<T> getJsonList(String path, Class<T> elementType) {
    JavaType listType = mapper.getTypeFactory().constructCollectionType(List.class, elementType);
    return readJson(sendGet(path), path, listType);
  }

  private byte[] sendGet(String path) {
    HttpRequest req =
        HttpRequest.newBuilder(URI.create(baseUrl + path))
            .header("X-Api-Key", apiKey)
            .header("User-Agent", USER_AGENT)
            .timeout(REQUEST_TIMEOUT)
            .GET()
            .build();
    return send(req, "GET", path);
  }

  private <T> T postJson(String path, Object body, Class<T> type) {
    String payload;
    try {
      payload = mapper.writeValueAsString(body);
    } catch (IOException e) {
      throw new OpenBankingException("could not serialize request body", e);
    }
    HttpRequest req =
        HttpRequest.newBuilder(URI.create(baseUrl + path))
            .header("X-Api-Key", apiKey)
            .header("Content-Type", "application/json")
            .header("User-Agent", USER_AGENT)
            .timeout(REQUEST_TIMEOUT)
            .POST(HttpRequest.BodyPublishers.ofString(payload, StandardCharsets.UTF_8))
            .build();
    byte[] data = send(req, "POST", path);
    return readJson(data, path, mapper.getTypeFactory().constructType(type));
  }

  private byte[] send(HttpRequest req, String method, String path) {
    HttpResponse<byte[]> resp;
    try {
      resp = http.send(req, HttpResponse.BodyHandlers.ofByteArray());
    } catch (IOException e) {
      throw new OpenBankingException(method + " " + path + " failed", e);
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
      throw new OpenBankingException(method + " " + path + " was interrupted", e);
    }
    if (resp.statusCode() < 200 || resp.statusCode() >= 300) {
      throw new OpenBankingException(method + " " + path + " failed: HTTP " + resp.statusCode());
    }
    return resp.body();
  }

  private <T> T readJson(byte[] data, String path, JavaType type) {
    try {
      return mapper.readValue(data, type);
    } catch (IOException e) {
      throw new OpenBankingException(path + " returned an unparseable body", e);
    }
  }

  private static void appendParam(StringBuilder qs, String key, String value) {
    if (value == null) {
      return;
    }
    qs.append(qs.length() == 0 ? '?' : '&')
        .append(key)
        .append('=')
        .append(URLEncoder.encode(value, StandardCharsets.UTF_8));
  }

  private static String encodePathSegment(String s) {
    return URLEncoder.encode(s, StandardCharsets.UTF_8).replace("+", "%20");
  }

  private static BigDecimal parseDecimal(String s) {
    return (s == null || s.isEmpty()) ? BigDecimal.ZERO : new BigDecimal(s);
  }

  private static BigDecimal parseDecimalNullable(String s) {
    return (s == null || s.isEmpty()) ? null : new BigDecimal(s);
  }

  private static boolean isBlank(String s) {
    return s == null || s.isBlank();
  }
}
