package io.openbanking;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.security.InvalidKeyException;
import java.security.KeyFactory;
import java.security.NoSuchAlgorithmException;
import java.security.PrivateKey;
import java.security.PublicKey;
import java.security.spec.ECPoint;
import java.security.spec.ECPublicKeySpec;
import java.security.spec.InvalidKeySpecException;
import java.security.spec.PKCS8EncodedKeySpec;
import java.util.Arrays;
import java.util.Base64;
import javax.crypto.Cipher;
import javax.crypto.KeyAgreement;
import javax.crypto.Mac;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;

/**
 * Decrypts open-banking.io's zero-knowledge data envelopes.
 *
 * <p>Scheme: ephemeral ECDH on NIST P-256 -&gt; HKDF-SHA256 -&gt; AES-256-GCM. Wire: {@code
 * version(1)=0x01 | ephemeralPublicKeyRaw(65) | nonce(12) | tag(16) | ciphertext}. Only the user's
 * private key can decrypt — the service stores ciphertext it cannot read.
 */
final class Envelope {

  private static final int VERSION = 0x01;
  private static final int POINT_LEN = 65;
  private static final int COORD_LEN = 32;
  private static final int NONCE_LEN = 12;
  private static final int TAG_LEN = 16;
  private static final int HEADER_LEN = 1 + POINT_LEN + NONCE_LEN + TAG_LEN;
  private static final byte[] HKDF_SALT = new byte[32]; // 32 zero bytes (must be exactly this).
  private static final byte[] HKDF_INFO = "bank.core.ci/zk/v1".getBytes(StandardCharsets.UTF_8);

  private Envelope() {}

  /** Loads a base64 PKCS#8 EC (P-256) private key. */
  static java.security.interfaces.ECPrivateKey loadPrivateKey(String privateKeyPkcs8B64) {
    try {
      byte[] der = Base64.getDecoder().decode(privateKeyPkcs8B64.trim());
      KeyFactory kf = KeyFactory.getInstance("EC");
      PrivateKey key = kf.generatePrivate(new PKCS8EncodedKeySpec(der));
      return (java.security.interfaces.ECPrivateKey) key;
    } catch (Exception e) {
      throw new OpenBankingException("invalid PKCS#8 private key", e);
    }
  }

  /** Decrypts the raw bytes of a zero-knowledge envelope. */
  static byte[] decrypt(java.security.interfaces.ECPrivateKey privateKey, byte[] envelope) {
    if (envelope.length < HEADER_LEN || (envelope[0] & 0xFF) != VERSION) {
      throw new OpenBankingException("invalid or unsupported envelope");
    }
    try {
      byte[] ephPub = Arrays.copyOfRange(envelope, 1, 1 + POINT_LEN);
      byte[] nonce = Arrays.copyOfRange(envelope, 1 + POINT_LEN, 1 + POINT_LEN + NONCE_LEN);
      byte[] tag = Arrays.copyOfRange(envelope, 1 + POINT_LEN + NONCE_LEN, HEADER_LEN);
      byte[] ciphertext = Arrays.copyOfRange(envelope, HEADER_LEN, envelope.length);

      PublicKey ephemeral = importRawPublicKey(privateKey, ephPub);
      KeyAgreement ka = KeyAgreement.getInstance("ECDH");
      ka.init(privateKey);
      ka.doPhase(ephemeral, true);
      byte[] shared = ka.generateSecret();

      byte[] key = hkdfSha256(shared, HKDF_SALT, HKDF_INFO, 32);

      // Java's GCM expects ciphertext||tag.
      byte[] ctWithTag = new byte[ciphertext.length + tag.length];
      System.arraycopy(ciphertext, 0, ctWithTag, 0, ciphertext.length);
      System.arraycopy(tag, 0, ctWithTag, ciphertext.length, tag.length);

      Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
      // `key` is derived at runtime via HKDF-SHA256 over the ECDH shared secret (above);
      // it is not a hard-coded credential.
      // nosemgrep: java.lang.security.crypto.hardcoded-secret-key-spec.hardcoded-secret-key-spec
      cipher.init(
          Cipher.DECRYPT_MODE,
          new SecretKeySpec(key, "AES"),
          new GCMParameterSpec(TAG_LEN * 8, nonce));
      return cipher.doFinal(ctWithTag);
    } catch (OpenBankingException e) {
      throw e;
    } catch (Exception e) {
      throw new OpenBankingException("envelope decryption failed", e);
    }
  }

  /** Decrypts a base64 envelope and deserializes its JSON payload. Null/blank in -&gt; null. */
  static <T> T decryptTo(
      java.security.interfaces.ECPrivateKey privateKey,
      String envelopeB64,
      ObjectMapper mapper,
      Class<T> type) {
    if (envelopeB64 == null || envelopeB64.isBlank()) {
      return null;
    }
    byte[] plaintext = decrypt(privateKey, Base64.getDecoder().decode(envelopeB64));
    try {
      return mapper.readValue(plaintext, type);
    } catch (Exception e) {
      throw new OpenBankingException("invalid envelope payload", e);
    }
  }

  /** Reconstructs a P-256 public key from a raw uncompressed SEC1 point (0x04 | X(32) | Y(32)). */
  private static PublicKey importRawPublicKey(
      java.security.interfaces.ECPrivateKey privateKey, byte[] raw)
      throws NoSuchAlgorithmException, InvalidKeySpecException {
    if (raw.length != POINT_LEN || (raw[0] & 0xFF) != 0x04) {
      throw new OpenBankingException("invalid ephemeral public key");
    }
    BigInteger x = new BigInteger(1, Arrays.copyOfRange(raw, 1, 1 + COORD_LEN));
    BigInteger y = new BigInteger(1, Arrays.copyOfRange(raw, 1 + COORD_LEN, POINT_LEN));
    ECPublicKeySpec spec = new ECPublicKeySpec(new ECPoint(x, y), privateKey.getParams());
    return KeyFactory.getInstance("EC").generatePublic(spec);
  }

  /** HKDF-SHA256 (RFC 5869): extract-then-expand. */
  private static byte[] hkdfSha256(byte[] ikm, byte[] salt, byte[] info, int length)
      throws NoSuchAlgorithmException, InvalidKeyException {
    Mac mac = Mac.getInstance("HmacSHA256");

    // Extract.
    mac.init(new SecretKeySpec(salt, "HmacSHA256"));
    byte[] prk = mac.doFinal(ikm);

    // Expand.
    mac.init(new SecretKeySpec(prk, "HmacSHA256"));
    byte[] okm = new byte[length];
    byte[] t = new byte[0];
    int pos = 0;
    for (int counter = 1; pos < length; counter++) {
      mac.update(t);
      mac.update(info);
      mac.update((byte) counter);
      t = mac.doFinal();
      int n = Math.min(t.length, length - pos);
      System.arraycopy(t, 0, okm, pos, n);
      pos += n;
    }
    return okm;
  }
}
