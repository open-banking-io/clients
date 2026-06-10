package io.openbanking;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

import java.util.List;

// Internal wire DTOs (what the API returns; sensitive fields are ciphertext) and the decrypted
// envelope payloads (the camelCase contract with the backend). All package-private.

@JsonIgnoreProperties(ignoreUnknown = true)
record AccountWire(
        String id,
        String aspspName,
        String aspspCountry,
        String currency,
        String accountType,
        String bic,
        boolean needsReconnect,
        List<BalanceWire> balances,
        String enc,
        String displayNameEnc,
        String uidEnc) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record BalanceWire(
        String type,
        String currency,
        String referenceDate,
        String enc) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record TransactionPageWire(List<TransactionWire> items, long total) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record TransactionWire(
        String id,
        String currency,
        String creditDebitIndicator,
        String status,
        String bookingDate,
        String valueDate,
        String transactionDate,
        String bankTransactionCode,
        String enc) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record ConnectionWire(
        String sessionId,
        String aspspName,
        String aspspCountry,
        String validUntil,
        String status,
        long accountCount,
        String lastSyncedAt,
        String psuType) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record SyncResultWire(long newTransactions, long totalFetched) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record SyncAllResultWire(long accounts, long newTransactions) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record AccountEnc(String ownerName, String iban, String bban, String accountName, String product) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record DisplayNameEnc(String displayName) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record UidEnc(String uid) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record BalanceEnc(String amount, String name) {
}

@JsonIgnoreProperties(ignoreUnknown = true)
record TransactionEnc(
        String amount,
        String creditorName,
        String creditorIban,
        String creditorBban,
        String creditorAgentBic,
        String debtorName,
        String debtorIban,
        String debtorBban,
        String debtorAgentBic,
        String remittanceInformation,
        String note,
        String referenceNumber,
        String exchangeRate,
        String merchantCategoryCode,
        String balanceAfter,
        String balanceAfterCurrency) {
}
