<?php

declare(strict_types=1);

namespace OpenBankingIO\Model;

/** A balance snapshot. The type is the ISO 20022 code (ITBD booked, ITAV available, ...). */
final class Balance
{
    public function __construct(
        public readonly string $type,
        public readonly ?string $name,
        /** Decimal string; never a float (exact money). Null when the envelope could not be read. */
        public readonly ?string $amount,
        public readonly string $currency,
        public readonly ?string $referenceDate,
        public readonly ?string $decryptError = null,
    ) {
    }

    public function isSealed(): bool
    {
        return $this->decryptError !== null;
    }
}
