package io.openbanking;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.Test;

/**
 * Public model invariants: null collections normalize to empty, and lists are defensively copied.
 */
class ModelsTest {

  @Test
  void accountNormalizesNullBalancesToEmptyList() {
    Account a =
        new Account(
            "id", "Lunar", "DK", "DKK", "CACC", "BIC", false, null, null, null, null, null, null,
            null);
    assertEquals(List.of(), a.balances());
  }

  @Test
  void accountDefensivelyCopiesBalances() {
    List<Balance> source = new ArrayList<>();
    source.add(new Balance("ITBD", null, null, "DKK", null));
    Account a =
        new Account(
            "id", "Lunar", "DK", "DKK", "CACC", "BIC", false, null, null, null, null, null, null,
            source);
    source.clear(); // mutating the source must not affect the account
    assertEquals(1, a.balances().size());
    assertThrows(UnsupportedOperationException.class, () -> a.balances().add(null));
  }

  @Test
  void transactionPageNormalizesNullItemsToEmptyList() {
    TransactionPage page = new TransactionPage(null, 0);
    assertEquals(List.of(), page.items());
  }
}
