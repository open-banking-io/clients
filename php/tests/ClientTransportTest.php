<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

use OpenBankingIO\ApiException;
use OpenBankingIO\Client;
use PHPUnit\Framework\TestCase;

/**
 * Covers the transport-override surface added in 0.3.0: the optional 4th constructor
 * argument `array $options` with `curl_options`, `timeout`, and `connect_timeout` keys.
 *
 * Uses the same PHP-built-in mock server as the integration tests, with an added
 * OBK_DELAY_MS knob so the request timeout can be driven against a slow server.
 */
final class ClientTransportTest extends TestCase
{
    use MockServerTrait;

    protected function tearDown(): void
    {
        $this->stopMockServer();
    }

    /**
     * @param array{curl_options?: array<int, mixed>, timeout?: int, connect_timeout?: int} $options
     */
    private function client(array $options = []): Client
    {
        return new Client(
            $this->baseUrl,
            $this->credentials['apiKey'],
            $this->credentials['encryptionKey']['privateKey'],
            $options,
        );
    }

    /**
     * A caller-supplied CURLOPT is applied last, winning over the SDK default:
     * overriding CURLOPT_USERAGENT is reflected in what the server received.
     */
    public function testCurlOptionsAreAppliedAndWin(): void
    {
        $this->startMockServer();

        $this->client([
            'curl_options' => [CURLOPT_USERAGENT => 'custom-agent/9.9'],
        ])->getAccounts();

        $raw = file_get_contents($this->captureFile);
        self::assertNotFalse($raw);
        /** @var array<string, mixed> $all */
        $all = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        self::assertSame('custom-agent/9.9', $all['userAgent'] ?? null);
    }

    /**
     * A transport-shaping option such as CURLOPT_PROXY is honored: pointing at a
     * dead proxy port makes the otherwise-successful request fail.
     */
    public function testCurlProxyOptionIsHonored(): void
    {
        $this->startMockServer();

        // A closed local port -- routing through it must make the request fail.
        $deadProxy = '127.0.0.1:' . $this->findFreePort();

        $this->expectException(ApiException::class);
        $this->client([
            'curl_options' => [CURLOPT_PROXY => $deadProxy],
        ])->getAccounts();
    }

    /**
     * The `timeout` override is honored: against a server that delays 2s, a 1s
     * timeout aborts the request (whereas the control call with no override,
     * which keeps the 30s SDK default, succeeds).
     */
    public function testTimeoutOverrideIsHonored(): void
    {
        $this->serverEnvOverrides = ['OBK_DELAY_MS' => '2000'];
        $this->startMockServer();

        // Control: no override -> default 30s timeout comfortably outlasts the 2s delay.
        $accounts = $this->client()->getAccounts();
        self::assertCount(1, $accounts);

        // Override: 1s total timeout is shorter than the 2s server delay -> aborts.
        $this->expectException(ApiException::class);
        $this->client(['timeout' => 1])->getAccounts();
    }

    /**
     * The `connect_timeout` override is honored: a 1s connect timeout against a
     * non-routable host fails well under the 10s SDK default.
     */
    public function testConnectTimeoutOverrideIsHonored(): void
    {
        // No mock server needed -- 198.51.100.1 is TEST-NET-2 (RFC 5737), non-routable.
        /** @var array{apiKey: string, encryptionKey: array{privateKey: string}} $credentials */
        $credentials = Fixtures::load('credentials.json');
        $client = new Client(
            'http://198.51.100.1:9',
            'api-key',
            $credentials['encryptionKey']['privateKey'],
            ['connect_timeout' => 1],
        );

        $start = microtime(true);
        try {
            $client->getAccounts();
            self::fail('Expected the connect attempt to fail');
        } catch (ApiException) {
            // expected
        }
        $elapsed = microtime(true) - $start;

        // A 1s connect timeout must abort well before the 10s SDK default would.
        self::assertLessThan(8.0, $elapsed);
    }
}
