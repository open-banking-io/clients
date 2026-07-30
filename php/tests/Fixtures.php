<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

/** Locates and loads the shared fixtures, in the monorepo or in the split-off package. */
final class Fixtures
{
    /**
     * The release pipeline subtree-splits php/ into its own repository, where the repo-root
     * fixtures/ of this monorepo does not exist, so it vendors them in beside the tests first.
     */
    public static function dir(): string
    {
        $vendored = __DIR__ . '/fixtures';

        return is_dir($vendored) ? $vendored : dirname(__DIR__, 2) . '/fixtures';
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
