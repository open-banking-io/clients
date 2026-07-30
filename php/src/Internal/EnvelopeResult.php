<?php

declare(strict_types=1);

namespace OpenBankingIO\Internal;

/**
 * The outcome of opening one zero-knowledge envelope.
 *
 * A sealed result means the ciphertext could not be read, which is not the same as the
 * plaintext being empty -- callers that treat the two alike will import a transaction
 * whose amount and counterparty silently became nothing.
 *
 * No public method accepts or returns one; the three states are surfaced on the models
 *           as a nullable value plus $decryptError.
 *
 * @internal Not part of the public API; it may change or move in any release.
 */
final class EnvelopeResult
{
    /**
     * @param array<string, mixed> $fields
     */
    private function __construct(
        public readonly array $fields,
        public readonly ?string $error,
        private readonly bool $present,
    ) {
    }

    /**
     * @param array<string, mixed> $fields
     */
    public static function opened(array $fields): self
    {
        return new self($fields, null, true);
    }

    public static function absent(): self
    {
        return new self([], null, false);
    }

    public static function sealed(string $reason): self
    {
        return new self([], $reason, true);
    }

    public function isReadable(): bool
    {
        return $this->present && $this->error === null;
    }

    public function isAbsent(): bool
    {
        return !$this->present;
    }

    /**
     * @param mixed $default
     * @return mixed
     */
    public function get(string $field, $default = null)
    {
        return $this->fields[$field] ?? $default;
    }
}
