package io.openbanking;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Map;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

/** apiBaseUrl normalization: whitespace, missing scheme and cleartext http. */
class BaseUrlTest {

  private static final Path FIXTURES = Path.of("..", "fixtures");

  @SuppressWarnings("unchecked")
  private static String privateKeyPkcs8() throws Exception {
    Map<String, Object> keypair =
        new ObjectMapper()
            .readValue(Files.readAllBytes(FIXTURES.resolve("keypair.json")), Map.class);
    return (String) keypair.get("privateKeyPkcs8B64");
  }

  private static OpenBankingClient build(String baseUrl) throws Exception {
    return new OpenBankingClient(baseUrl, "k", privateKeyPkcs8(), null);
  }

  @ParameterizedTest
  @ValueSource(
      strings = {"  https://example.test", "https://example.test  ", "\thttps://example.test\n"})
  @DisplayName("whitespace around the base url is stripped")
  void trimsWhitespace(String padded) {
    assertDoesNotThrow(() -> build(padded));
  }

  @ParameterizedTest
  @ValueSource(strings = {"open-banking.io", "//open-banking.io", "ftp://open-banking.io"})
  @DisplayName("a base url without an http scheme is rejected")
  void rejectsMissingScheme(String bad) {
    OpenBankingException e = assertThrows(OpenBankingException.class, () -> build(bad));
    assertTrue(e.getMessage().contains("http"), e.getMessage());
  }

  @ParameterizedTest
  @ValueSource(
      strings = {
        "http://open-banking.io",
        "http://192.168.1.10:8080",
        "http://localhost.evil.test"
      })
  @DisplayName("cleartext http to a remote host is rejected")
  void rejectsCleartextToRemoteHost(String bad) {
    OpenBankingException e = assertThrows(OpenBankingException.class, () -> build(bad));
    assertTrue(e.getMessage().contains("https"), e.getMessage());
  }

  @ParameterizedTest
  @ValueSource(strings = {"http://localhost:8080", "http://127.0.0.1:8080", "http://[::1]:8080"})
  @DisplayName("cleartext http is allowed on loopback")
  void allowsCleartextOnLoopback(String ok) {
    assertDoesNotThrow(() -> build(ok));
  }

  @ParameterizedTest
  @ValueSource(strings = {"", "   "})
  @DisplayName("a blank base url is rejected")
  void rejectsBlank(String blank) {
    OpenBankingException e = assertThrows(OpenBankingException.class, () -> build(blank));
    assertTrue(e.getMessage().contains("required"), e.getMessage());
  }
}
