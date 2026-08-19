<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

use OpenBankingIO\ApiException;
use OpenBankingIO\Client;
use OpenBankingIO\Internal\EnvelopeResult;
use PHPUnit\Framework\TestCase;

/**
 * A 200 whose body is not the agreed shape has to arrive as an ApiException. Callers catch that;
 * a TypeError escaping from inside the client reaches their generic handler instead, where a
 * rejected key looks the same as a network blip.
 */
final class ResponseShapeTest extends TestCase
{
    use MockServerTrait;
    use SealsEnvelopes;

    protected function tearDown(): void
    {
        $this->stopMockServer();
    }

    public function testABareScalarBodyIsAnApiException(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'scalar-body'];
        $this->startMockServer();

        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('returned int');

        $this->client()->getAccounts();
    }

    public function testAnObjectWhereAListIsExpectedIsAnApiException(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'object-body'];
        $this->startMockServer();

        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('expected a list of objects');

        $this->client()->getAccounts();
    }

    public function testASyncResponseWithoutItsCountersIsAnApiException(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'sync-no-counters'];
        $this->startMockServer();

        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('expected accounts');

        $this->client()->syncAll();
    }

    /**
     * The service negotiates a window the bank refuses DOWN rather than failing outright, so the
     * served window is the only proof of what a backfill actually covered. A caller that asked for
     * two years and silently received ninety days has to be able to see that.
     */
    public function testSyncSurfacesTheWindowTheServiceActuallyServed(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'sync-narrowed-window'];
        $this->startMockServer();

        $result = $this->client()->sync(
            '11111111-1111-4111-8111-111111111111',
            ['fromDate' => '2024-01-01'],
        );

        self::assertSame('2026-05-19', $result->servedFromDate);
    }

    public function testAFleetWithOneTornSessionSyncsTheRestAndNamesTheCasualty(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'mixed-fleet'];
        $this->startMockServer();

        $result = $this->client()->syncAll();

        // The readable account still syncs; the torn one is named with its reason; the account
        // with no session at all is simply not connected and does not appear.
        self::assertFalse($result->isComplete());
        self::assertSame(['22222222-2222-4222-8222-222222222222'], array_keys($result->unreadable));
        self::assertNotSame('', $result->unreadable['22222222-2222-4222-8222-222222222222']);
        self::assertSame(1, $result->accounts);
    }

    public function testAFleetWhereNoSessionCanBeReadThrowsRatherThanReportingSuccess(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'sealed-session'];
        $this->startMockServer();

        $this->expectException(\OpenBankingIO\OpenBankingException::class);
        $this->expectExceptionMessage('nothing was synced');

        $this->client()->syncAll();
    }

    public function testAnEnvelopeFieldThatIsNotAStringIsSealedNotAbsent(): void
    {
        $this->startMockServer();

        $open = new \ReflectionMethod(Client::class, 'openEnvelope');

        $absent = $open->invoke($this->client(), null);
        $sealed = $open->invoke($this->client(), 12345);

        self::assertInstanceOf(EnvelopeResult::class, $absent);
        self::assertInstanceOf(EnvelopeResult::class, $sealed);
        self::assertNull($absent->error, 'a missing envelope is not a failure');
        self::assertNotNull($sealed->error, 'a malformed envelope field must not read as "no data"');
    }

    public function testAKeyedMapWhereAListIsExpectedIsAnApiException(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'keyed-body'];
        $this->startMockServer();

        // A keyed object satisfies is_array() and loops once over its single value, so without a
        // list check the row validation runs against the wrong container.
        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('expected a list of objects');

        $this->client()->getAccounts();
    }

    public function testANonIntegerTotalIsAnApiExceptionRatherThanZero(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'bad-total'];
        $this->startMockServer();

        // Coercing it would report a finished, empty page and let the caller's window advance.
        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('total is array');

        $this->client()->getTransactions('11111111-1111-4111-8111-111111111111');
    }

    public function testASessionThatOpensWithoutAUidIsReportedTheSameWayAsAsealedOne(): void
    {
        $credentials = Fixtures::load('credentials.json');
        self::assertIsArray($credentials['encryptionKey']);
        $key = $credentials['encryptionKey']['privateKey'];
        self::assertIsString($key);

        $this->useRecipientKey($key);

        // sync() throws for this account, so syncAll() must not quietly leave it out of the run.
        $this->serverEnvOverrides = [
            'OBK_ACCOUNTS_MODE' => 'uidless-session',
            'OBK_UIDLESS_ENC' => $this->seal('{"notauid":"x"}'),
        ];
        $this->startMockServer();

        $this->expectException(\OpenBankingIO\OpenBankingException::class);
        $this->expectExceptionMessage('no usable uid');

        $this->client()->syncAll();
    }

    public function testATotalPastTheIntegerRangeIsAnApiExceptionRatherThanSaturated(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'huge-total'];
        $this->startMockServer();

        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('out of range');

        $this->client()->getTransactions('11111111-1111-4111-8111-111111111111');
    }

    public function testAListWhoseEntriesAreNotObjectsIsAnApiException(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'scalar-rows'];
        $this->startMockServer();

        // The top-level container is a list, so only the per-row check catches this.
        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('expected a list of objects');

        $this->client()->getAccounts();
    }

    public function testATransactionsPageMissingItemsOrTotalIsAnApiException(): void
    {
        foreach (['no-total', 'no-items'] as $mode) {
            $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => $mode];
            $this->startMockServer();

            try {
                $this->client()->getTransactions('11111111-1111-4111-8111-111111111111');
                self::fail("{$mode} should not have produced a page");
            } catch (ApiException $e) {
                // Without the presence check this is an undefined-key warning and a TypeError,
                // neither of which a caller catching ApiException sees.
                self::assertStringContainsString('expected items and total', $e->getMessage());
            } finally {
                $this->stopMockServer();
            }
        }
    }

    public function testEveryUnreadableAccountIsListedEvenWhenIdsRepeatOrAreMissing(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'twin-sealed'];
        $this->startMockServer();

        try {
            $this->client()->syncAll();
            self::fail('nothing was readable, so this should have thrown');
        } catch (\OpenBankingIO\OpenBankingException $e) {
            // Keying purely by id would have collapsed four faults into two.
            self::assertCount(4, array_unique(explode('; ', substr($e->getMessage(), (int) strpos($e->getMessage(), ': ') + 2))));
            self::assertStringContainsString('(account with no id)', $e->getMessage());
        }
    }

    public function testAnAccountSaysWhichOfItsEnvelopesFailed(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'sealed-display-name'];
        $this->startMockServer();

        $account = $this->client()->getAccounts()[0];

        // Without the label a display-name failure is indistinguishable from the account's own
        // fields being unreadable, which is what a caller would act on.
        self::assertTrue($account->isSealed());
        self::assertStringStartsWith('displayName: ', (string) $account->decryptError);
    }

    public function testASealReasonSaysWhatWentWrong(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'sealed-session'];
        $this->startMockServer();

        try {
            $this->client()->syncAll();
            self::fail('expected the sealed fleet to throw');
        } catch (\OpenBankingIO\OpenBankingException $e) {
            self::assertMatchesRegularExpression('/AES-256-GCM|ephemeral|envelope|base64/i', $e->getMessage());
        }
    }

    public function testATotalWithATrailingNewlineIsNotQuietlyCoerced(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'newline-total'];
        $this->startMockServer();

        // PCRE's $ also matches before a final newline, so this needs the /D anchor.
        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('expected an integer');

        $this->client()->getTransactions('11111111-1111-4111-8111-111111111111');
    }

    public function testTheLargestTotalTheIntegerRangeHoldsIsAccepted(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'edge-total'];
        $this->startMockServer();

        // Exercises the equal-length comparison, which a length-only check would never reach.
        self::assertSame(PHP_INT_MAX, $this->client()->getTransactions('11111111-1111-4111-8111-111111111111')->total);
    }

    public function testATotalOnePastTheIntegerRangeIsRejected(): void
    {
        $this->serverEnvOverrides = ['OBK_ACCOUNTS_MODE' => 'over-edge-total'];
        $this->startMockServer();

        $this->expectException(ApiException::class);
        $this->expectExceptionMessage('out of range');

        $this->client()->getTransactions('11111111-1111-4111-8111-111111111111');
    }

    private function client(): Client
    {
        return new Client(
            $this->baseUrl,
            $this->credentials['apiKey'],
            $this->credentials['encryptionKey']['privateKey'],
        );
    }
}
