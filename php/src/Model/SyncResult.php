<?php

declare(strict_types=1);

namespace OpenBankingIO\Model;

/** The result of syncing a single account. */
final class SyncResult
{
    public function __construct(
        public readonly int $newTransactions,
        public readonly int $totalFetched,
    ) {
    }
}
