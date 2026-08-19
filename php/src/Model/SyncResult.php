<?php

declare(strict_types=1);

namespace OpenBankingIO\Model;

/** The result of syncing a single account. */
final class SyncResult
{
    /**
     * @param string|null $servedFromDate The window the service actually fetched from (YYYY-MM-DD),
     *                                    which is not necessarily the one that was asked for: a bank
     *                                    caps how far into the past a request may reach, and the
     *                                    service negotiates a refused window DOWN rather than failing.
     *                                    Null when no backfill window was requested.
     */
    public function __construct(
        public readonly int $newTransactions,
        public readonly int $totalFetched,
        public readonly ?string $servedFromDate = null,
    ) {
    }
}
