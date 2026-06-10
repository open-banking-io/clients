package io.openbanking;

/** The result of syncing one account. */
public record SyncResult(long newTransactions, long totalFetched) {
}
