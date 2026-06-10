package io.openbanking;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

/** The encryption key section of a credentials bundle; {@code privateKey} is base64 PKCS#8. */
@JsonIgnoreProperties(ignoreUnknown = true)
public record EncryptionKey(
        String scheme,
        String curve,
        String privateKeyFormat,
        String privateKey,
        String publicKey) {
}
