<?php

declare(strict_types=1);

namespace OpenBankingIO\Model;

/** A page of transactions, newest first. */
final class TransactionPage
{
    /**
     * @param Transaction[] $items
     */
    public function __construct(
        public readonly array $items,
        public readonly int $total,
    ) {
    }
}
