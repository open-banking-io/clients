package io.openbanking;

import java.math.BigDecimal;

/** A statement transaction with its sensitive fields decrypted. */
public record Transaction(
    String id,
    String currency,
    String creditDebitIndicator,
    String status,
    String bookingDate,
    String valueDate,
    String transactionDate,
    String bankTransactionCode,
    BigDecimal amount,
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
    BigDecimal balanceAfterTransaction,
    String balanceAfterCurrency) {}
