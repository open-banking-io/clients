<?php

declare(strict_types=1);

namespace OpenBankingIO\Model;

/**
 * A statement transaction with its sensitive fields decrypted. Amounts are decimal strings.
 *
 * A null $amount means the envelope could not be read, never that the transaction was for
 * nothing -- $decryptError says which. Importing such a row books a zero-value entry.
 */
final class Transaction
{
    public function __construct(
        public readonly string $id,
        public readonly string $currency,
        public readonly string $creditDebitIndicator,
        public readonly ?string $status,
        public readonly ?string $bookingDate,
        public readonly ?string $valueDate,
        public readonly ?string $transactionDate,
        public readonly ?string $bankTransactionCode,
        public readonly ?string $amount,
        public readonly ?string $creditorName,
        public readonly ?string $creditorIban,
        public readonly ?string $creditorBban,
        public readonly ?string $creditorAgentBic,
        public readonly ?string $debtorName,
        public readonly ?string $debtorIban,
        public readonly ?string $debtorBban,
        public readonly ?string $debtorAgentBic,
        public readonly ?string $remittanceInformation,
        public readonly ?string $note,
        public readonly ?string $referenceNumber,
        public readonly ?string $exchangeRate,
        public readonly ?string $merchantCategoryCode,
        public readonly ?string $balanceAfter,
        public readonly ?string $balanceAfterCurrency,
        public readonly ?string $rawJson,
        public readonly ?string $decryptError = null,
    ) {
    }

    public function isSealed(): bool
    {
        return $this->decryptError !== null;
    }
}
