package io.openbanking;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import java.io.IOException;
import java.math.BigDecimal;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/** Integration tests against a mock API served from the shared fixtures. */
class IntegrationTest {

  private static final Path FIXTURES = Path.of("..", "fixtures");
  private static final ObjectMapper MAPPER = new ObjectMapper();

  private HttpServer server;
  private String baseUrl;
  private String apiKey;
  private String privateKey;
  private Map<String, Object> lastSyncBody;
  private String lastUserAgent;

  private static byte[] fixture(String name) throws IOException {
    return Files.readAllBytes(FIXTURES.resolve(name));
  }

  @SuppressWarnings("unchecked")
  private static Map<String, Object> readJson(String name) throws IOException {
    return MAPPER.readValue(fixture(name), Map.class);
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

  private void handle(HttpExchange ex) throws IOException {
    try {
      if (!apiKey.equals(ex.getRequestHeaders().getFirst("X-Api-Key"))) {
        respond(ex, 401, "{\"error\":\"unauthorized\"}".getBytes(StandardCharsets.UTF_8));
        return;
      }
      lastUserAgent = ex.getRequestHeaders().getFirst("User-Agent");
      String method = ex.getRequestMethod();
      String path = ex.getRequestURI().getPath();

      if (method.equals("GET") && path.equals("/api/accounts")) {
        respond(ex, 200, fixture("api/accounts.json"));
      } else if (method.equals("GET") && path.matches("^/api/accounts/[^/]+/transactions$")) {
        respond(ex, 200, fixture("api/transactions.json"));
      } else if (method.equals("GET") && path.equals("/api/connections")) {
        respond(ex, 200, fixture("api/connections.json"));
      } else if (method.equals("POST") && path.matches("^/api/accounts/[^/]+/sync$")) {
        captureBody(ex);
        respond(ex, 200, fixture("api/sync.json"));
      } else if (method.equals("POST") && path.equals("/api/sync")) {
        captureBody(ex);
        respond(ex, 200, fixture("api/sync-all.json"));
      } else {
        respond(ex, 404, "{\"error\":\"not found\"}".getBytes(StandardCharsets.UTF_8));
      }
    } catch (Exception e) {
      respond(ex, 500, ("{\"error\":\"" + e.getMessage() + "\"}").getBytes(StandardCharsets.UTF_8));
    }
  }

  @SuppressWarnings("unchecked")
  private void captureBody(HttpExchange ex) throws IOException {
    byte[] body = ex.getRequestBody().readAllBytes();
    lastSyncBody = MAPPER.readValue(body, Map.class);
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

  @Test
  void getAccountsDecryptsFields() {
    List<Account> accounts = client().getAccounts();
    assertEquals(1, accounts.size());
    Account a = accounts.get(0);
    assertEquals("DK6466952001724927", a.iban());
    assertEquals("Tatic ApS", a.ownerName());
    assertEquals("Drift", a.displayName());
    assertEquals(3, a.balances().size());

    Map<String, BigDecimal> byType = new java.util.HashMap<>();
    for (Balance b : a.balances()) {
      byType.put(b.type(), b.amount());
    }
    assertEquals(0, new BigDecimal("828.13").compareTo(byType.get("ITBD")));
    assertEquals(0, new BigDecimal("633.90").compareTo(byType.get("ITAV")));
  }

  @Test
  void getTransactionsDecryptsAmountAndCreditor() {
    TransactionPage page =
        client()
            .getTransactions(
                "11111111-1111-4111-8111-111111111111", TransactionQuery.none().withLimit(50));
    assertEquals(1, page.total());
    Transaction t = page.items().get(0);
    assertEquals(0, new BigDecimal("194.23").compareTo(t.amount()));
    assertEquals("One.com", t.creditorName());
  }

  @Test
  void getConnectionsWorks() {
    List<Connection> conns = client().getConnections();
    assertEquals(1, conns.size());
    assertEquals("Lunar", conns.get(0).aspspName());
    assertEquals("Active", conns.get(0).status());
  }

  @Test
  void syncPostsDecryptedUid() {
    SyncResult result = client().sync("11111111-1111-4111-8111-111111111111");
    assertEquals(1, result.totalFetched());
    assertNotNull(lastSyncBody);
    assertEquals("c5d93aa7-5e23-4da0-ba88-42b9a584492c", lastSyncBody.get("uid"));
  }

  @Test
  @SuppressWarnings("unchecked")
  void syncAllPostsItemsWithDecryptedUid() {
    SyncAllResult result = client().syncAll();
    assertEquals(1, result.accounts());
    assertNotNull(lastSyncBody);
    List<Map<String, Object>> items = (List<Map<String, Object>>) lastSyncBody.get("items");
    assertEquals(1, items.size());
    assertEquals("11111111-1111-4111-8111-111111111111", items.get(0).get("accountId"));
    assertEquals("c5d93aa7-5e23-4da0-ba88-42b9a584492c", items.get(0).get("uid"));
  }

  @Test
  void sendsUserAgentHeader() {
    client().getAccounts();
    assertNotNull(lastUserAgent);
    assertTrue(
        lastUserAgent.startsWith("open-banking-io/java/"),
        "unexpected User-Agent: " + lastUserAgent);
  }

  @Test
  void aWrongApiKeyThrows() {
    OpenBankingClient bad = new OpenBankingClient(baseUrl, "wrong-key", privateKey, null);
    assertThrows(OpenBankingException.class, bad::getAccounts);
  }

  @Test
  void aWrongPrivateKeyThrowsOnDecryption() {
    String badKey =
        "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgdB9vjCwdrF1FckSNDHrw8M7PNNkpUB/tAK/EgAbZWvihRANCAASgg8XOKlU9VeYCee9+tQKtDSkFze10CRTA4b2gGKDlHIFw+QTf1AkjAjLfWqCJ4BctUqQtAYs0v0Y90Bw1wsBF";
    OpenBankingClient bad = new OpenBankingClient(baseUrl, apiKey, badKey, null);
    assertThrows(OpenBankingException.class, bad::getAccounts);
  }
}
