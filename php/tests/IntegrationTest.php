<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

use OpenBankingIO\ApiException;
use OpenBankingIO\Client;
use PHPUnit\Framework\TestCase;

/**
 * Integration test: serve the shared API fixtures over HTTP via the PHP built-in server
 * and exercise the client end to end (HTTP + decryption).
 */
final class IntegrationTest extends TestCase
{
    private const ACCOUNT_ID = '11111111-1111-4111-8111-111111111111';

    /** @var resource|null */
    private $serverProcess;
    private string $baseUrl = '';
    private string $captureFile = '';
    /** @var array{apiKey: string, encryptionKey: array{privateKey: string}} */
    private array $credentials;

    protected function setUp(): void
    {
        /** @var array{apiKey: string, encryptionKey: array{privateKey: string}} $credentials */
        $credentials = Fixtures::load('credentials.json');
        $this->credentials = $credentials;

        $port = $this->findFreePort();
        $this->baseUrl = "http://127.0.0.1:{$port}";
        $this->captureFile = tempnam(sys_get_temp_dir(), 'obk-capture-') ?: '';

        $router = __DIR__ . '/mock-server/router.php';
        $env = [
            'OBK_FIXTURES' => Fixtures::dir(),
            'OBK_API_KEY' => $this->credentials['apiKey'],
            'OBK_CAPTURE_FILE' => $this->captureFile,
            'PATH' => getenv('PATH') ?: '',
        ];

        $descriptors = [
            0 => ['pipe', 'r'],
            1 => ['pipe', 'w'],
            2 => ['pipe', 'w'],
        ];

        $cmd = [PHP_BINARY, '-S', "127.0.0.1:{$port}", $router];
        $process = proc_open($cmd, $descriptors, $pipes, null, $env);
        if (!is_resource($process)) {
            self::fail('Could not start the PHP mock server');
        }
        $this->serverProcess = $process;

        $this->waitForServer();
    }

    protected function tearDown(): void
    {
        if (is_resource($this->serverProcess)) {
            proc_terminate($this->serverProcess);
            proc_close($this->serverProcess);
        }
        if ($this->captureFile !== '' && is_file($this->captureFile)) {
            @unlink($this->captureFile);
        }
    }

    private function client(?string $apiKey = null): Client
    {
        return new Client(
            $this->baseUrl,
            $apiKey ?? $this->credentials['apiKey'],
            $this->credentials['encryptionKey']['privateKey'],
        );
    }

    public function testGetAccountsDecrypts(): void
    {
        $accounts = $this->client()->getAccounts();

        self::assertCount(1, $accounts);
        $acc = $accounts[0];
        self::assertSame('DK6466952001724927', $acc->iban);
        self::assertSame('Tatic ApS', $acc->ownerName);
        self::assertSame('Drift', $acc->displayName);
        self::assertSame('Lunar', $acc->aspspName);

        $byType = [];
        foreach ($acc->balances as $b) {
            $byType[$b->type] = $b;
        }
        self::assertSame('828.13', $byType['ITBD']->amount);
        self::assertSame('633.90', $byType['ITAV']->amount);
    }

    public function testSendsUserAgentHeader(): void
    {
        $this->client()->getAccounts();

        $raw = file_get_contents($this->captureFile);
        self::assertNotFalse($raw);
        /** @var array<string, mixed> $all */
        $all = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        $userAgent = $all['userAgent'] ?? null;
        self::assertIsString($userAgent);
        self::assertStringStartsWith('open-banking-io/php/', $userAgent);
    }

    public function testGetTransactionsDecrypts(): void
    {
        $page = $this->client()->getTransactions(self::ACCOUNT_ID, ['limit' => 50]);

        self::assertSame(1, $page->total);
        $txn = $page->items[0];
        self::assertSame('194.23', $txn->amount);
        self::assertSame('One.com', $txn->creditorName);
        self::assertSame('4816', $txn->merchantCategoryCode);
        self::assertSame('633.90', $txn->balanceAfter);
        self::assertSame('DBIT', $txn->creditDebitIndicator);
    }

    public function testGetConnections(): void
    {
        $connections = $this->client()->getConnections();

        self::assertCount(1, $connections);
        $conn = $connections[0];
        self::assertSame('22222222-2222-4222-8222-222222222222', $conn->sessionId);
        self::assertSame('Lunar', $conn->aspspName);
        self::assertSame(1, $conn->accountCount);
        self::assertSame('business', $conn->psuType);
    }

    public function testSyncPostsDecryptedUid(): void
    {
        $result = $this->client()->sync(self::ACCOUNT_ID);

        self::assertSame(1, $result->totalFetched);
        $body = $this->captured('sync');
        self::assertSame(['uid' => 'c5d93aa7-5e23-4da0-ba88-42b9a584492c'], $body);
    }

    public function testSyncAllPostsDecryptedUids(): void
    {
        $result = $this->client()->syncAll();

        self::assertSame(1, $result->accounts);
        /** @var array{items: array<int, array{uid: string, accountId: string}>} $body */
        $body = $this->captured('syncAll');
        self::assertSame('c5d93aa7-5e23-4da0-ba88-42b9a584492c', $body['items'][0]['uid']);
        self::assertSame(self::ACCOUNT_ID, $body['items'][0]['accountId']);
    }

    public function testWrongApiKeyRaises(): void
    {
        $this->expectException(ApiException::class);
        $this->client('wrong-key')->getAccounts();
    }

    public function testWrongPrivateKeyRaises(): void
    {
        $other = openssl_pkey_new([
            'private_key_type' => OPENSSL_KEYTYPE_EC,
            'curve_name' => 'prime256v1',
        ]);
        self::assertNotFalse($other);
        $pem = '';
        openssl_pkey_export($other, $pem);
        self::assertIsString($pem);
        $wrongB64 = preg_replace('/-----[^-]+-----|\s+/', '', $pem) ?? '';

        $client = new Client($this->baseUrl, $this->credentials['apiKey'], $wrongB64);
        $this->expectException(\OpenBankingIO\OpenBankingException::class);
        $client->getAccounts();
    }

    // -- helpers ---------------------------------------------------------------

    /**
     * @return array<string, mixed>
     */
    private function captured(string $label): array
    {
        $raw = file_get_contents($this->captureFile);
        self::assertNotFalse($raw);
        /** @var array<string, mixed> $all */
        $all = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        $body = $all[$label];
        self::assertIsArray($body);
        /** @var array<string, mixed> $body */
        return $body;
    }

    private function findFreePort(): int
    {
        $sock = stream_socket_server('tcp://127.0.0.1:0', $errno, $errstr);
        if ($sock === false) {
            self::fail("Could not allocate a port: {$errstr}");
        }
        $name = stream_socket_get_name($sock, false);
        fclose($sock);
        $port = (int) substr((string) $name, (int) strrpos((string) $name, ':') + 1);
        return $port;
    }

    private function waitForServer(): void
    {
        for ($i = 0; $i < 100; $i++) {
            $conn = @fsockopen('127.0.0.1', (int) parse_url($this->baseUrl, PHP_URL_PORT), $errno, $errstr, 0.2);
            if ($conn !== false) {
                fclose($conn);
                return;
            }
            usleep(50_000);
        }
        self::fail('Mock server did not start in time');
    }
}
