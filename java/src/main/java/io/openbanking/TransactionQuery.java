package io.openbanking;

/** Optional filters for a transactions page; any field may be {@code null}. */
public record TransactionQuery(String from, String to, Integer limit, Integer offset) {

  /** An empty query (no filters). */
  public static TransactionQuery none() {
    return new TransactionQuery(null, null, null, null);
  }

  public TransactionQuery withFrom(String value) {
    return new TransactionQuery(value, to, limit, offset);
  }

  public TransactionQuery withTo(String value) {
    return new TransactionQuery(from, value, limit, offset);
  }

  public TransactionQuery withLimit(Integer value) {
    return new TransactionQuery(from, to, value, offset);
  }

  public TransactionQuery withOffset(Integer value) {
    return new TransactionQuery(from, to, limit, value);
  }
}
