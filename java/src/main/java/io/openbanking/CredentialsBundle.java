package io.openbanking;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

/** The credentials bundle exported from open-banking.io (API key + encryption private key). */
@JsonIgnoreProperties(ignoreUnknown = true)
public record CredentialsBundle(
        String service,
        String apiBaseUrl,
        String user,
        String apiKey,
        EncryptionKey encryptionKey) {
}
