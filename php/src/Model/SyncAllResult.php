<?php

declare(strict_types=1);

namespace OpenBankingIO\Model;

/**
 * The result of syncing every account with an active session.
 *
 * $unreadable maps an account id to why its session could not be used -- an envelope that failed
 * to decrypt, or one that opened without a usable uid. Those accounts were not synced and are not
 * counted; an empty $unreadable is the only proof the run was complete. An account with no session
 * at all is simply not connected and does not appear.
 */
final class SyncAllResult
{
    /**
     * @param array<string, string> $unreadable
     */
    public function __construct(
        public readonly int $accounts,
        public readonly int $newTransactions,
        public readonly array $unreadable = [],
    ) {
    }

    public function isComplete(): bool
    {
        return $this->unreadable === [];
    }
}
