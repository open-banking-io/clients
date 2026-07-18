package io.openbanking;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.http.HttpClient;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.regex.Pattern;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * Unit tests for {@link OpenBankingClient}'s request-building, factory, and error-handling paths,
 * driven by a programmable loopback server. These complement {@link IntegrationTest} (happy-path
 * decryption against the shared fixtures) by exercising the branches it does not: input validation,
 * the {@code fromCredentials}/{@code fromBundle} factories, query encoding, sync error paths, null
 * ciphertext fields, and HTTP/transport failures.
 */
class OpenBankingClientTest {

  private static final Path FIXTURES = Path.of("..", "fixtures");
  private static final ObjectMapper MAPPER = new ObjectMapper();

  private record Route(String method, Pattern path, int status, byte[] body) {}

  private HttpServer server;
  private String baseUrl;
  private String apiKey;
  private String privateKey;
  private final List<Route> routes = new ArrayList<>();
  private String lastQuery;
  private byte[] lastBody;

  @SuppressWarnings("unchecked")
  private static Map<String, Object> readJson(String name) throws IOException {
    return MAPPER.readValue(Files.readAllBytes(FIXTURES.resolve(name)), Map.class);
  }

  @BeforeEach
  void setUp() throws Exception {
    apiKey = (String) readJson("credentials.json").get("apiKey");
    privateKey = (String) readJson("keypair.json").get("privateKeyPkcs8B64");

    server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
    server.createContext("/", this::handle);
    server.start();
    baseUrl = "http://127.0.0.1:" + server.getAddress().getPort();
  }

  @AfterEach
  void tearDown() {
    server.stop(0);
  }

  /**
   * Registers a canned response for requests matching {@code method} and the {@code path} regex.
   */
  private void on(String method, String pathRegex, int status, String body) {
    routes.add(
        new Route(
            method, Pattern.compile(pathRegex), status, body.getBytes(StandardCharsets.UTF_8)));
  }

  private void handle(HttpExchange ex) throws IOException {
    lastQuery = ex.getRequestURI().getRawQuery();
    lastBody = ex.getRequestBody().readAllBytes();
    String method = ex.getRequestMethod();
    String path = ex.getRequestURI().getPath();
    for (Route r : routes) {
      if (r.method().equals(method) && r.path().matcher(path).matches()) {
        respond(ex, r.status(), r.body());
        return;
      }
    }
    respond(ex, 404, "{\"error\":\"not found\"}".getBytes(StandardCharsets.UTF_8));
  }

  private static void respond(HttpExchange ex, int status, byte[] body) throws IOException {
    ex.getResponseHeaders().set("Content-Type", "application/json");
    ex.sendResponseHeaders(status, body.length);
    ex.getResponseBody().write(body);
    ex.close();
  }

  private OpenBankingClient client() {
    return new OpenBankingClient(baseUrl, apiKey, privateKey, null);
  }

  // ---- constructor validation ----

  @Test
  void blankApiBaseUrlThrows() {
    OpenBankingException ex =
        assertThrows(
            OpenBankingException.class,
            () -> new OpenBankingClient("  ", apiKey, privateKey, null));
    assertTrue(ex.getMessage().contains("apiBaseUrl"));
  }

  @Test
  void blankApiKeyThrows() {
    OpenBankingException ex =
        assertThrows(
            OpenBankingException.class,
            () -> new OpenBankingClient(baseUrl, null, privateKey, null));
    assertTrue(ex.getMessage().contains("apiKey"));
  }

  @Test
  void blankPrivateKeyThrows() {
    OpenBankingException ex =
        assertThrows(
            OpenBankingException.class, () -> new OpenBankingClient(baseUrl, apiKey, "", null));
    assertTrue(ex.getMessage().contains("privateKey"));
  }

  @Test
  void trailingSlashesAreStrippedFromBaseUrl() {
    on("GET", "/api/accounts", 200, "[]");
    OpenBankingClient c = new OpenBankingClient(baseUrl + "///", apiKey, privateKey, null);
    assertEquals(0, c.getAccounts().size());
  }

  @Test
  void injectedHttpClientIsUsed() {
    on("GET", "/api/accounts", 200, "[]");
    OpenBankingClient c =
        new OpenBankingClient(baseUrl, apiKey, privateKey, HttpClient.newHttpClient());
    assertEquals(0, c.getAccounts().size());
  }

  @Test
  void shortRequestTimeoutAgainstHungServerFails() throws Exception {
    // A dedicated server whose handler blocks until the test releases it — far longer than the
    // client's per-request timeout — so the client must abort with an OpenBankingException rather
    // than wait it out. The latch lets teardown be immediate (no fixed sleep) so the test is fast.
    CountDownLatch release = new CountDownLatch(1);
    HttpServer slow = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
    slow.createContext(
        "/",
        ex -> {
          try {
            release.await();
          } catch (InterruptedException ie) {
            Thread.currentThread().interrupt();
          }
          respond(ex, 200, "[]".getBytes(StandardCharsets.UTF_8));
        });
    slow.start();
    try {
      String slowUrl = "http://127.0.0.1:" + slow.getAddress().getPort();
      OpenBankingClient c =
          new OpenBankingClient(slowUrl, apiKey, privateKey, null, Duration.ofMillis(150));
      OpenBankingException ex = assertThrows(OpenBankingException.class, c::getAccounts);
      assertTrue(ex.getMessage().contains("failed"));
    } finally {
      release.countDown();
      slow.stop(0);
    }
  }

  // ---- fromCredentials / fromBundle factories ----

  private String bundleJson() {
    return "{\"apiBaseUrl\":\""
        + baseUrl
        + "\",\"apiKey\":\""
        + apiKey
        + "\",\"encryptionKey\":{\"privateKey\":\""
        + privateKey
        + "\"}}";
  }

  @Test
  void fromCredentialsAcceptsJsonString() {
    on("GET", "/api/accounts", 200, "[]");
    OpenBankingClient c = OpenBankingClient.fromCredentials(bundleJson());
    assertEquals(0, c.getAccounts().size());
  }

  @Test
  void fromCredentialsReadsBundleFromFile() throws Exception {
    Path file = Files.createTempFile("bundle", ".json");
    Files.writeString(file, bundleJson());
    on("GET", "/api/accounts", 200, "[]");
    OpenBankingClient c = OpenBankingClient.fromCredentials(file.toString());
    assertEquals(0, c.getAccounts().size());
    Files.deleteIfExists(file);
  }

  @Test
  void fromCredentialsMissingFileThrows() {
    OpenBankingException ex =
        assertThrows(
            OpenBankingException.class,
            () -> OpenBankingClient.fromCredentials("/no/such/credentials.json"));
    assertTrue(ex.getMessage().contains("could not read credentials file"));
  }

  @Test
  void fromCredentialsMalformedJsonThrows() {
    OpenBankingException ex =
        assertThrows(
            OpenBankingException.class,
            () -> OpenBankingClient.fromCredentials("{ not valid json"));
    assertTrue(ex.getMessage().contains("could not parse the credentials bundle"));
  }

  @Test
  void fromBundleNullBundleThrows() {
    assertThrows(OpenBankingException.class, () -> OpenBankingClient.fromBundle(null, null));
  }

  @Test
  void fromBundleWithoutApiKeyThrows() {
    CredentialsBundle b = new CredentialsBundle(null, baseUrl, null, "  ", null);
    OpenBankingException ex =
        assertThrows(OpenBankingException.class, () -> OpenBankingClient.fromBundle(b, null));
    assertTrue(ex.getMessage().contains("no apiKey"));
  }

  @Test
  void fromBundleWithoutEncryptionKeyThrows() {
    CredentialsBundle b = new CredentialsBundle(null, baseUrl, null, apiKey, null);
    OpenBankingException ex =
        assertThrows(OpenBankingException.class, () -> OpenBankingClient.fromBundle(b, null));
    assertTrue(ex.getMessage().contains("encryption private key"));

    EncryptionKey blankKey = new EncryptionKey(null, null, null, " ", null);
    CredentialsBundle b2 = new CredentialsBundle(null, baseUrl, null, apiKey, blankKey);
    assertThrows(OpenBankingException.class, () -> OpenBankingClient.fromBundle(b2, null));
  }

  // ---- query building ----

  @Test
  void nullQuerySendsNoParameters() {
    on("GET", "/api/accounts/[^/]+/transactions", 200, "{\"items\":[],\"total\":0}");
    client().getTransactions("acc-1", null);
    assertNull(lastQuery);
  }

  @Test
  void allQueryParametersAreEncoded() {
    on("GET", "/api/accounts/[^/]+/transactions", 200, "{\"items\":[],\"total\":0}");
    TransactionQuery q =
        TransactionQuery.none()
            .withFrom("2026-01-01")
            .withTo("2026-02-01")
            .withLimit(50)
            .withOffset(10);
    client().getTransactions("acc-1", q);
    assertEquals("from=2026-01-01&to=2026-02-01&limit=50&offset=10", lastQuery);
  }

  @Test
  void accountIdWithSpacesIsPathEncoded() {
    on("GET", "/api/accounts/[^/]+/transactions", 200, "{\"items\":[],\"total\":0}");
    // A space must become %20 (not '+') in the path segment.
    client().getTransactions("a b", TransactionQuery.none());
    // The route still matches because %20 is a single non-slash segment; nothing to assert beyond
    // the request succeeding without throwing.
  }

  @Test
  void pageWithoutItemsFieldYieldsEmptyList() {
    on("GET", "/api/accounts/[^/]+/transactions", 200, "{\"total\":0}");
    TransactionPage page = client().getTransactions("acc-1", TransactionQuery.none());
    assertEquals(0, page.items().size());
    assertEquals(0, page.total());
  }

  // ---- null ciphertext fields decrypt to null, never throw ----

  @Test
  void accountWithNullCiphertextFieldsDecryptsToNulls() {
    String accounts =
        "[{\"id\":\"acc-1\",\"aspspName\":\"Lunar\",\"aspspCountry\":\"DK\",\"currency\":\"DKK\","
            + "\"accountType\":\"CACC\",\"bic\":\"LUNADK22\",\"needsReconnect\":false,"
            + "\"balances\":null,\"enc\":null,\"displayNameEnc\":null,\"uidEnc\":null}]";
    on("GET", "/api/accounts", 200, accounts);
    List<Account> result = client().getAccounts();
    assertEquals(1, result.size());
    Account a = result.get(0);
    assertEquals("acc-1", a.id());
    assertNull(a.iban());
    assertNull(a.ownerName());
    assertNull(a.displayName());
    assertEquals(0, a.balances().size());
  }

  @Test
  void balanceWithNullCiphertextDecryptsToNullName() {
    String accounts =
        "[{\"id\":\"acc-1\",\"aspspName\":\"Lunar\",\"aspspCountry\":\"DK\",\"currency\":\"DKK\","
            + "\"accountType\":\"CACC\",\"bic\":\"LUNADK22\",\"needsReconnect\":false,"
            + "\"balances\":[{\"type\":\"ITBD\",\"currency\":\"DKK\",\"referenceDate\":null,"
            + "\"enc\":null}],\"enc\":null,\"displayNameEnc\":null,\"uidEnc\":null}]";
    on("GET", "/api/accounts", 200, accounts);
    Account a = client().getAccounts().get(0);
    assertEquals(1, a.balances().size());
    Balance b = a.balances().get(0);
    assertEquals("ITBD", b.type());
    assertNull(b.name());
    // A null amount ciphertext parses as BigDecimal.ZERO.
    assertEquals(0, java.math.BigDecimal.ZERO.compareTo(b.amount()));
  }

  @Test
  void transactionWithNullCiphertextDecryptsToNulls() {
    String txns =
        "{\"items\":[{\"id\":\"tx-1\",\"currency\":\"DKK\",\"creditDebitIndicator\":\"DBIT\","
            + "\"status\":\"BOOK\",\"bookingDate\":\"2026-01-01\",\"valueDate\":null,"
            + "\"transactionDate\":null,\"bankTransactionCode\":null,\"enc\":null}],\"total\":1}";
    on("GET", "/api/accounts/[^/]+/transactions", 200, txns);
    Transaction t = client().getTransactions("acc-1", TransactionQuery.none()).items().get(0);
    assertEquals("tx-1", t.id());
    assertNull(t.creditorName());
    assertNull(t.balanceAfterTransaction());
    // amount falls back to ZERO; balanceAfterTransaction (nullable) stays null.
    assertEquals(0, java.math.BigDecimal.ZERO.compareTo(t.amount()));
  }

  // ---- sync error paths ----

  @Test
  void syncUnknownAccountThrows() {
    on("GET", "/api/accounts", 200, "[]");
    OpenBankingException ex =
        assertThrows(OpenBankingException.class, () -> client().sync("missing"));
    assertTrue(ex.getMessage().contains("not found"));
  }

  @Test
  void syncAccountWithoutActiveSessionThrows() {
    String accounts =
        "[{\"id\":\"acc-1\",\"aspspName\":\"Lunar\",\"aspspCountry\":\"DK\",\"currency\":\"DKK\","
            + "\"accountType\":\"CACC\",\"bic\":\"LUNADK22\",\"needsReconnect\":true,"
            + "\"balances\":null,\"enc\":null,\"displayNameEnc\":null,\"uidEnc\":null}]";
    on("GET", "/api/accounts", 200, accounts);
    OpenBankingException ex =
        assertThrows(OpenBankingException.class, () -> client().sync("acc-1"));
    assertTrue(ex.getMessage().contains("no active session"));
  }

  @Test
  void syncAllSkipsAccountsWithoutSession() {
    String accounts =
        "[{\"id\":\"acc-1\",\"aspspName\":\"Lunar\",\"aspspCountry\":\"DK\",\"currency\":\"DKK\","
            + "\"accountType\":\"CACC\",\"bic\":\"LUNADK22\",\"needsReconnect\":true,"
            + "\"balances\":null,\"enc\":null,\"displayNameEnc\":null,\"uidEnc\":null}]";
    on("GET", "/api/accounts", 200, accounts);
    on("POST", "/api/sync", 200, "{\"accounts\":0,\"newTransactions\":0}");
    SyncAllResult result = client().syncAll();
    assertEquals(0, result.accounts());
    // The posted body must carry an empty items list (every account was skipped).
    Map<String, Object> posted = MAPPER.convertValue(parse(lastBody), Map.class);
    assertEquals(List.of(), posted.get("items"));
  }

  @SuppressWarnings("unchecked")
  private static Map<String, Object> parse(byte[] body) {
    try {
      return MAPPER.readValue(body, Map.class);
    } catch (IOException e) {
      throw new RuntimeException(e);
    }
  }

  // ---- HTTP / transport failures ----

  @Test
  void non2xxStatusThrows() {
    on("GET", "/api/accounts", 500, "{\"error\":\"boom\"}");
    OpenBankingException ex =
        assertThrows(OpenBankingException.class, () -> client().getAccounts());
    assertTrue(ex.getMessage().contains("HTTP 500"));
  }

  @Test
  void unparseableBodyThrows() {
    on("GET", "/api/connections", 200, "this is not json");
    OpenBankingException ex =
        assertThrows(OpenBankingException.class, () -> client().getConnections());
    assertTrue(ex.getMessage().contains("unparseable body"));
  }

  @Test
  void transportFailureThrows() {
    // Port 1 is privileged and unbound in the test sandbox: the connection is refused, surfacing
    // as an IOException that the client must wrap.
    OpenBankingClient c = new OpenBankingClient("http://127.0.0.1:1", apiKey, privateKey, null);
    OpenBankingException ex = assertThrows(OpenBankingException.class, c::getAccounts);
    assertTrue(ex.getMessage().contains("failed"));
  }
}
