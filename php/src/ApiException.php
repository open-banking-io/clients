<?php

declare(strict_types=1);

namespace OpenBankingIO;

/** Thrown when the open-banking.io API returns a non-2xx response or the request fails. */
final class ApiException extends OpenBankingException
{
    public function __construct(
        string $message,
        public readonly int $statusCode = 0,
        public readonly ?string $responseBody = null,
    ) {
        parent::__construct($message);
    }
}
