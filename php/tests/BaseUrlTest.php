<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

use OpenBankingIO\Client;
use OpenBankingIO\OpenBankingException;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/** apiBaseUrl normalization: whitespace, missing scheme and cleartext http. */
final class BaseUrlTest extends TestCase
{
    use MockServerTrait;

    protected function tearDown(): void
    {
        $this->stopMockServer();
    }

    /** @return list<array{string}> */
    public static function paddingProvider(): array
    {
        return [['  %s'], ['%s  '], ["\t%s\n"], ['  %s/  ']];
    }

    #[DataProvider('paddingProvider')]
    public function testWhitespacePaddedBaseUrlStillReachesTheServer(string $pattern): void
    {
        $this->startMockServer();
        /** @var array{apiKey: string, encryptionKey: array{privateKey: string}} $creds */
        $creds = Fixtures::load('credentials.json');
        $client = new Client(
            sprintf($pattern, $this->baseUrl),
            $creds['apiKey'],
            $creds['encryptionKey']['privateKey'],
        );
        $this->assertNotEmpty($client->getAccounts());
    }

    /** @return list<array{string}> */
    public static function noSchemeProvider(): array
    {
        return [['open-banking.io'], ['//open-banking.io'], ['ftp://open-banking.io']];
    }

    #[DataProvider('noSchemeProvider')]
    public function testBaseUrlWithoutHttpSchemeIsRejected(string $bad): void
    {
        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessageMatches('/http/i');
        new Client($bad, 'k', 'x');
    }

    /** @return list<array{string}> */
    public static function cleartextProvider(): array
    {
        return [
            ['http://open-banking.io'],
            ['http://192.168.1.10:8080'],
            ['http://localhost.evil.test'],
        ];
    }

    #[DataProvider('cleartextProvider')]
    public function testCleartextHttpToRemoteHostIsRejected(string $bad): void
    {
        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessageMatches('/https/i');
        new Client($bad, 'k', 'x');
    }

    public function testEmptyBaseUrlIsRejected(): void
    {
        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessageMatches('/required/');
        new Client('   ', 'k', 'x');
    }
}
