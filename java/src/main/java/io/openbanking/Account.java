package io.openbanking;

import java.util.List;

/** A bank account with its sensitive fields decrypted. */
public record Account(
        String id,
        String aspspName,
        String aspspCountry,
        String currency,
        String accountType,
        String bic,
        boolean needsReconnect,
        String iban,
        String bban,
        String ownerName,
        String accountName,
        String product,
        String displayName,
        List<Balance> balances) {
}
