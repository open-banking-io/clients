package io.openbanking;

import java.util.List;

/** A page of transactions, newest first. */
public record TransactionPage(List<Transaction> items, long total) {
}
