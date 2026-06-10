<?php

declare(strict_types=1);

namespace OpenBankingIO\Model;

/** A bank connection (consent). */
final class Connection
{
    public function __construct(
        public readonly string $sessionId,
        public readonly string $aspspName,
        public readonly string $aspspCountry,
        public readonly ?string $validUntil,
        public readonly string $status,
        public readonly int $accountCount,
        public readonly ?string $lastSyncedAt,
        public readonly ?string $psuType,
    ) {
    }
}
