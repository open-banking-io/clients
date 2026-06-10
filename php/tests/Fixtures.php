<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

/** Locates and loads the shared fixtures at the repo-root fixtures/ directory. */
final class Fixtures
{
    /** php/tests/ -> php/ -> repo root -> fixtures/ */
    public static function dir(): string
    {
        return dirname(__DIR__, 2) . '/fixtures';
    }

    /**
     * @return array<string, mixed>|array<int, mixed>
     */
    public static function load(string ...$parts): array
    {
        $path = self::dir() . '/' . implode('/', $parts);
        $raw = file_get_contents($path);
        if ($raw === false) {
            throw new \RuntimeException("Could not read fixture: {$path}");
        }
        /** @var array<string, mixed>|array<int, mixed> $decoded */
        $decoded = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        return $decoded;
    }
}
