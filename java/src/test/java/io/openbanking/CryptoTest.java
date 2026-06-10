package io.openbanking;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

import java.nio.file.Files;
import java.nio.file.Path;
import java.security.interfaces.ECPrivateKey;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;

/** Envelope decryption (wire-format interop), verified against the shared fixtures. */
class CryptoTest {

    private static final Path FIXTURES = Path.of("..", "fixtures");
    private static final ObjectMapper MAPPER = new ObjectMapper();

    @SuppressWarnings("unchecked")
    private static Map<String, Object> readJson(String name) throws Exception {
        return MAPPER.readValue(Files.readAllBytes(FIXTURES.resolve(name)), Map.class);
    }

    private static ECPrivateKey privateKey() throws Exception {
        return Envelope.loadPrivateKey((String) readJson("keypair.json").get("privateKeyPkcs8B64"));
    }

    @SuppressWarnings("unchecked")
    private static Map<String, Object> decrypt(String envB64) throws Exception {
        return Envelope.decryptTo(privateKey(), envB64, MAPPER, Map.class);
    }

    @Test
    void decryptsAccountEnvelope() throws Exception {
        Map<String, Object> envelopes = readJson("envelopes.json");
        Map<String, Object> expected = (Map<String, Object>) readJson("expected.json").get("account");
        Map<String, Object> acc = decrypt((String) envelopes.get("account"));
        assertEquals(expected.get("iban"), acc.get("iban"));
        assertEquals(expected.get("ownerName"), acc.get("ownerName"));
        assertEquals(expected.get("bban"), acc.get("bban"));
    }

    @Test
    @SuppressWarnings("unchecked")
    void decryptsDisplayNameEnvelope() throws Exception {
        Map<String, Object> envelopes = readJson("envelopes.json");
        Map<String, Object> expected = (Map<String, Object>) readJson("expected.json").get("displayName");
        Map<String, Object> dn = decrypt((String) envelopes.get("displayName"));
        assertEquals(expected.get("displayName"), dn.get("displayName"));
    }

    @Test
    @SuppressWarnings("unchecked")
    void decryptsUidEnvelope() throws Exception {
        Map<String, Object> envelopes = readJson("envelopes.json");
        Map<String, Object> expected = (Map<String, Object>) readJson("expected.json").get("uid");
        Map<String, Object> uid = decrypt((String) envelopes.get("uid"));
        assertEquals(expected.get("uid"), uid.get("uid"));
    }

    @Test
    @SuppressWarnings("unchecked")
    void decryptsBalanceEnvelopeKeepingAmountAsString() throws Exception {
        Map<String, Object> envelopes = readJson("envelopes.json");
        Map<String, Object> balances = (Map<String, Object>) readJson("expected.json").get("balances");
        Map<String, Object> itbd = (Map<String, Object>) balances.get("ITBD");
        Map<String, Object> bal = decrypt((String) envelopes.get("balance"));
        assertInstanceOf(String.class, bal.get("amount"));
        assertEquals(itbd.get("amount"), bal.get("amount"));
        assertEquals(itbd.get("name"), bal.get("name"));
    }

    @Test
    @SuppressWarnings("unchecked")
    void decryptsTransactionEnvelope() throws Exception {
        Map<String, Object> envelopes = readJson("envelopes.json");
        Map<String, Object> expected = (Map<String, Object>) readJson("expected.json").get("transaction");
        Map<String, Object> tx = decrypt((String) envelopes.get("transaction"));
        assertInstanceOf(String.class, tx.get("amount"));
        assertEquals(expected.get("amount"), tx.get("amount"));
        assertEquals(expected.get("creditorName"), tx.get("creditorName"));
    }

    @Test
    void aWrongKeyFailsToDecrypt() throws Exception {
        // A different valid P-256 PKCS#8 key derives the wrong shared secret.
        ECPrivateKey bad = Envelope.loadPrivateKey(
                "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgdB9vjCwdrF1FckSNDHrw8M7PNNkpUB/tAK/EgAbZWvihRANCAASgg8XOKlU9VeYCee9+tQKtDSkFze10CRTA4b2gGKDlHIFw+QTf1AkjAjLfWqCJ4BctUqQtAYs0v0Y90Bw1wsBF");
        Map<String, Object> envelopes = readJson("envelopes.json");
        assertThrows(OpenBankingException.class,
                () -> Envelope.decryptTo(bad, (String) envelopes.get("account"), MAPPER, Map.class));
    }
}
