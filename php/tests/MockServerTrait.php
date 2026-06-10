<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

/**
 * Boots the shared fixtures over the PHP built-in server (the mock open-banking.io API)
 * and tears it down again. Shared by the integration tests so the server-bootstrap
 * mechanics live in exactly one place.
 *
 * Consumers may set $serverEnvOverrides before setUp() runs (e.g. OBK_ACCOUNTS_MODE)
 * to exercise the router's negative-path payloads.
 */
trait MockServerTrait
{
    /** @var resource|null */
    private $serverProcess;
    private string $baseUrl = '';
    private string $captureFile = '';
    /** @var array{apiKey: string, encryptionKey: array{privateKey: string}} */
    private array $credentials;
    /** @var array<string, string> */
    private array $serverEnvOverrides = [];

    protected function startMockServer(): void
    {
        /** @var array{apiKey: string, encryptionKey: array{privateKey: string}} $credentials */
        $credentials = Fixtures::load('credentials.json');
        $this->credentials = $credentials;

        $port = $this->findFreePort();
        $this->baseUrl = "http://127.0.0.1:{$port}";
        $this->captureFile = tempnam(sys_get_temp_dir(), 'obk-capture-') ?: '';

        $router = __DIR__ . '/mock-server/router.php';
        $env = array_merge([
            'OBK_FIXTURES' => Fixtures::dir(),
            'OBK_API_KEY' => $this->credentials['apiKey'],
            'OBK_CAPTURE_FILE' => $this->captureFile,
            'PATH' => getenv('PATH') ?: '',
        ], $this->serverEnvOverrides);

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

    protected function stopMockServer(): void
    {
        if (is_resource($this->serverProcess)) {
            proc_terminate($this->serverProcess);
            proc_close($this->serverProcess);
        }
        if ($this->captureFile !== '' && is_file($this->captureFile)) {
            @unlink($this->captureFile);
        }
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
