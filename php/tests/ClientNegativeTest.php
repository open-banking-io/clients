<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

use OpenBankingIO\ApiException;
use OpenBankingIO\Client;
use OpenBankingIO\OpenBankingException;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * Negative-path integration coverage against the mock server: HTTP error statuses,
 * unparseable response bodies, transport failures, and the sync paths that depend on
 * an account's decrypted session uid (not found / no active session / sessionless skip).
 */
final class ClientNegativeTest extends TestCase
{
    use MockServerTrait;

    private const ACCOUNT_ID = '11111111-1111-4111-8111-111111111111';

    protected function tearDown(): void
    {
        $this->stopMockServer();
    }

    private function client(): Client
    {
        return new Client(
            $this->baseUrl,
            $this->credentials['apiKey'],
            $this->credentials['encryptionKey']['privateKey'],
        );
    }

    // -- HTTP error paths ------------------------------------------------------

    public function testNon2xxStatusRaisesApiException(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'error-status'];
        $this->startMockServer();

        try {
            $this->client()->getConnections();
            self::fail('Expected ApiException');
        } catch (ApiException $e) {
            self::assertSame(503, $e->statusCode);
            self::assertIsString($e->responseBody);
            self::assertStringContainsString('service unavailable', $e->responseBody);
        }
    }

    public function testInvalidJsonResponseRaisesApiException(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'bad-json'];
        $this->startMockServer();

        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('returned invalid JSON');
        $this->client()->getConnections();
    }

    public function testTransportFailureRaisesApiException(): void
    {
        // Bind a client to a port nothing is listening on so the connection is refused.
        $client = new Client(
            'http://127.0.0.1:1',
            'irrelevant',
            (function (): string {
                /** @var array{encryptionKey: array{privateKey: string}} $c */
                $c = Fixtures::load('credentials.json');
                return $c['encryptionKey']['privateKey'];
            })(),
        );

        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('failed');
        $client->getConnections();
    }

    // -- sync paths ------------------------------------------------------------

    public function testSyncUnknownAccountRaises(): void
    {
        $this->startMockServer();

        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('not found');
        $this->client()->sync('does-not-exist');
    }

    public function testSyncAccountWithoutSessionRaises(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'sessionless'];
        $this->startMockServer();

        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('no active session');
        $this->client()->sync(self::ACCOUNT_ID);
    }

    /**
     * A malformed window is refused locally, before the account lookup and before any bank work:
     * the service answers a bad fromDate with a 400 whose body says nothing about which argument
     * was wrong, and on this endpoint a round trip can cost minutes at a slow bank.
     */
    #[DataProvider('malformedWindows')]
    public function testSyncRefusesAMalformedBackfillWindow(string $fromDate): void
    {
        $this->startMockServer();

        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('fromDate must be a calendar date');
        $this->client()->sync(self::ACCOUNT_ID, ['fromDate' => $fromDate]);
    }

    /**
     * A non-string window has to arrive as an OpenBankingException too. `$opts` is a plain array,
     * so nothing stops a caller passing a DateTime or an integer date, and a raw TypeError out of
     * the SDK reaches their generic handler where it looks like a transport fault.
     */
    public function testSyncRefusesANonStringBackfillWindow(): void
    {
        $this->startMockServer();

        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('got: DateTimeImmutable');
        /** @phpstan-ignore argument.type */
        $this->client()->sync(self::ACCOUNT_ID, ['fromDate' => new \DateTimeImmutable('2026-08-01')]);
    }

    /** @return array<string, array{string}> */
    public static function malformedWindows(): array
    {
        return [
            'not a date' => ['last tuesday'],
            'wrong order' => ['01-08-2026'],
            'no zero padding' => ['2026-8-1'],
            'day out of range' => ['2026-02-31'],
            'month out of range' => ['2026-13-01'],
            'timestamp' => ['2026-08-01T00:00:00Z'],
            'trailing newline' => ["2026-08-01\n"],
        ];
    }

    public function testSyncAllSkipsSessionlessAccounts(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'sessionless'];
        $this->startMockServer();

        // The only account has no uid, so no items are posted; the call still succeeds.
        $result = $this->client()->syncAll();
        self::assertSame(1, $result->accounts);

        $raw = file_get_contents($this->captureFile);
        self::assertNotFalse($raw);
        /** @var array{syncAll?: array{items?: array<int, mixed>}} $all */
        $all = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        self::assertSame([], $all['syncAll']['items'] ?? null);
    }
}
