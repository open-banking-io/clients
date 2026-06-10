package io.openbanking;

/** The result of syncing every account with an active session. */
public record SyncAllResult(long accounts, long newTransactions) {}
