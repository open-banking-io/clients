package io.openbanking;

/** A bank connection (consent). */
public record Connection(
        String sessionId,
        String aspspName,
        String aspspCountry,
        String validUntil,
        String status,
        long accountCount,
        String lastSyncedAt,
        String psuType) {
}
