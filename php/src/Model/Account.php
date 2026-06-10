<?php

declare(strict_types=1);

namespace OpenBankingIO\Model;

/** A bank account with its sensitive fields decrypted. */
final class Account
{
    /**
     * @param Balance[] $balances
     */
    public function __construct(
        public readonly string $id,
        public readonly string $aspspName,
        public readonly string $aspspCountry,
        public readonly string $currency,
        public readonly ?string $accountType,
        public readonly ?string $bic,
        public readonly bool $needsReconnect,
        public readonly ?string $iban,
        public readonly ?string $bban,
        public readonly ?string $ownerName,
        public readonly ?string $accountName,
        public readonly ?string $product,
        public readonly ?string $displayName,
        public readonly array $balances,
    ) {
    }
}
