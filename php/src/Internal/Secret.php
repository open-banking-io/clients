<?php

declare(strict_types=1);

namespace OpenBankingIO\Internal;

/**
 * Holds a credential where dumping the object that owns it cannot reach it.
 *
 * #[\SensitiveParameter] redacts stack frames only, and __debugInfo() is honoured by var_dump()
 * and print_r() but not by Symfony's VarDumper -- which Laravel's dd()/dump() and every
 * Ignition/Flare-style crash reporter are built on. VarDumper emits real properties alongside
 * __debugInfo(), and it dumps a closure's captured variables, so neither a property nor a closure
 * is safe. Statics are not dumped, so the value lives in one keyed off the instance.
 *
 *
 * @internal Not part of the public API; it may change or move in any release.
 */
final class Secret
{
    /** @var \WeakMap<self, string>|null */
    private static ?\WeakMap $values = null;

    public function __construct(#[\SensitiveParameter] string $value)
    {
        self::$values ??= new \WeakMap();
        self::$values[$this] = $value;
    }

    public function reveal(): string
    {
        $value = self::$values[$this] ?? null;

        if (!is_string($value)) {
            throw new \OpenBankingIO\OpenBankingException('This secret was copied or restored and no longer holds a value');
        }

        return $value;
    }

    /**
     * @return array<string, string>
     */
    public function __debugInfo(): array
    {
        return ['value' => '[redacted]'];
    }

    public function __toString(): string
    {
        return '[redacted]';
    }

    /**
     * @return never
     */
    public function __serialize(): array
    {
        throw new \OpenBankingIO\OpenBankingException('A Secret must not be serialised');
    }

    /**
     * @param array<array-key, mixed> $data
     * @return never
     */
    public function __unserialize(array $data): void
    {
        throw new \OpenBankingIO\OpenBankingException('A Secret must not be serialised');
    }
}
