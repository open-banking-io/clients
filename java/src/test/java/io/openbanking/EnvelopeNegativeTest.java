package io.openbanking;

import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.interfaces.ECPrivateKey;
import java.util.Arrays;
import java.util.Base64;
import java.util.Map;
import org.junit.jupiter.api.Test;

/**
 * Negative / robustness tests for the zero-knowledge envelope wire format and key loading.
 *
 * <p>Every malformed input must surface as an {@link OpenBankingException}, never a raw {@code
 * ArrayIndexOutOfBoundsException}, {@code ClassCastException}, or silent garbage.
 */
class EnvelopeNegativeTest {

  private static final Path FIXTURES = Path.of("..", "fixtures");
  private static final ObjectMapper MAPPER = new ObjectMapper();

  @SuppressWarnings("unchecked")
  private static Map<String, Object> readJson(String name) throws Exception {
    return MAPPER.readValue(Files.readAllBytes(FIXTURES.resolve(name)), Map.class);
  }

  private static ECPrivateKey privateKey() throws Exception {
    return Envelope.loadPrivateKey((String) readJson("keypair.json").get("privateKeyPkcs8B64"));
  }

  /** The raw bytes of the (valid) account envelope from the shared fixtures. */
  private static byte[] validEnvelopeBytes() throws Exception {
    String b64 = (String) readJson("envelopes.json").get("account");
    return Base64.getDecoder().decode(b64);
  }

  @Test
  void emptyEnvelopeIsRejected() throws Exception {
    ECPrivateKey key = privateKey();
    OpenBankingException ex =
        assertThrows(OpenBankingException.class, () -> Envelope.decrypt(key, new byte[0]));
    assertTrue(ex.getMessage().contains("invalid or unsupported envelope"));
  }

  @Test
  void truncatedEnvelopeTooShortForHeaderIsRejected() throws Exception {
    ECPrivateKey key = privateKey();
    // A buffer shorter than the fixed header (1 + 65 + 12 + 16 = 94 bytes) must be rejected
    // up front, not indexed into.
    byte[] tooShort = new byte[10];
    tooShort[0] = 0x01; // correct version, but still too short
    OpenBankingException ex =
        assertThrows(OpenBankingException.class, () -> Envelope.decrypt(key, tooShort));
    assertTrue(ex.getMessage().contains("invalid or unsupported envelope"));
  }

  @Test
  void wrongVersionByteIsRejected() throws Exception {
    ECPrivateKey key = privateKey();
    byte[] envelope = validEnvelopeBytes();
    envelope[0] = 0x02; // bump the version byte to an unsupported value
    OpenBankingException ex =
        assertThrows(OpenBankingException.class, () -> Envelope.decrypt(key, envelope));
    assertTrue(ex.getMessage().contains("invalid or unsupported envelope"));
  }

  @Test
  void zeroVersionByteIsRejected() throws Exception {
    ECPrivateKey key = privateKey();
    byte[] envelope = validEnvelopeBytes();
    envelope[0] = 0x00;
    assertThrows(OpenBankingException.class, () -> Envelope.decrypt(key, envelope));
  }

  @Test
  void garbageEphemeralPublicKeyIsRejected() throws Exception {
    ECPrivateKey key = privateKey();
    byte[] envelope = validEnvelopeBytes();
    // The ephemeral point lives at bytes [1, 66). Byte 1 must be 0x04 (uncompressed SEC1).
    // Corrupt the leading point-format tag so it is no longer a valid uncompressed point.
    envelope[1] = 0x05;
    assertThrows(OpenBankingException.class, () -> Envelope.decrypt(key, envelope));
  }

  @Test
  void ephemeralPublicKeyNotOnCurveIsRejected() throws Exception {
    ECPrivateKey key = privateKey();
    byte[] envelope = validEnvelopeBytes();
    // Keep the 0x04 tag but fill the X/Y coordinates (point is 1 + 65 bytes) with bytes that
    // do not lie on P-256.
    int pointLen = 65;
    Arrays.fill(envelope, 2, 1 + pointLen, (byte) 0x11);
    assertThrows(OpenBankingException.class, () -> Envelope.decrypt(key, envelope));
  }

  @Test
  void tamperedCiphertextFailsAuthentication() throws Exception {
    ECPrivateKey key = privateKey();
    byte[] envelope = validEnvelopeBytes();
    // Flip the last ciphertext byte: AES-GCM tag verification must fail.
    envelope[envelope.length - 1] ^= 0x01;
    assertThrows(OpenBankingException.class, () -> Envelope.decrypt(key, envelope));
  }

  @Test
  void invalidPkcs8KeyIsRejected() {
    OpenBankingException ex =
        assertThrows(
            OpenBankingException.class, () -> Envelope.loadPrivateKey("not-base64-or-a-key!!!"));
    assertTrue(ex.getMessage().contains("invalid PKCS#8 private key"));
  }

  @Test
  void wellFormedBase64ButNotAKeyIsRejected() {
    // Valid base64 that decodes to bytes which are not a PKCS#8 EC key.
    String junk = Base64.getEncoder().encodeToString(new byte[] {1, 2, 3, 4, 5, 6, 7, 8});
    assertThrows(OpenBankingException.class, () -> Envelope.loadPrivateKey(junk));
  }

  @Test
  void malformedBase64EnvelopePayloadIsRejected() throws Exception {
    ECPrivateKey key = privateKey();
    // decryptTo base64-decodes first; non-base64 input must fail, not be silently dropped.
    assertThrows(
        IllegalArgumentException.class,
        () -> Envelope.decryptTo(key, "%%%not base64%%%", MAPPER, Map.class));
  }
}
