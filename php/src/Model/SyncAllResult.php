<?php

declare(strict_types=1);

namespace OpenBankingIO\Model;

/** The result of syncing every account with an active session. */
final class SyncAllResult
{
    public function __construct(
        public readonly int $accounts,
        public readonly int $newTransactions,
    ) {
    }
}
