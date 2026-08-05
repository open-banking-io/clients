<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

use OpenBankingIO\Client;
use OpenBankingIO\OpenBankingException;
use PHPUnit\Framework\TestCase;

/**
 * Covers the construction surface: constructor argument validation and the
 * Client::fromCredentials bundle loader (JSON string, file path, and every
 * malformed / missing-field branch). No network is touched.
 */
final class ClientFactoryTest extends TestCase
{
    /** @var array{apiKey: string, encryptionKey: array{privateKey: string}} */
    private array $credentials;
    private string $privateKey;

    protected function setUp(): void
    {
        /** @var array{apiKey: string, encryptionKey: array{privateKey: string}} $credentials */
        $credentials = Fixtures::load('credentials.json');
        $this->credentials = $credentials;
        $this->privateKey = $credentials['encryptionKey']['privateKey'];
    }

    // -- Constructor validation -----------------------------------------------

    public function testBlankBaseUrlRejected(): void
    {
        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('apiBaseUrl is required');
        new Client('   ', $this->credentials['apiKey'], $this->privateKey);
    }

    public function testBlankApiKeyRejected(): void
    {
        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('apiKey is required');
        new Client('https://api.example.com', '  ', $this->privateKey);
    }

    public function testBlankPrivateKeyRejected(): void
    {
        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('privateKeyPkcs8 is required');
        new Client('https://api.example.com', $this->credentials['apiKey'], '');
    }

    // -- fromCredentials: happy paths -----------------------------------------

    public function testFromCredentialsJsonString(): void
    {
        $json = json_encode([
            'apiBaseUrl' => 'https://api.example.com',
            'apiKey' => $this->credentials['apiKey'],
            'encryptionKey' => ['privateKey' => $this->privateKey],
        ], JSON_THROW_ON_ERROR);

        $client = Client::fromCredentials($json);
        self::assertInstanceOf(Client::class, $client);
    }

    /** The bundle's private key may be carried under privateKeyPkcs8B64 instead. */
    public function testFromCredentialsAcceptsPkcs8B64KeyField(): void
    {
        $json = json_encode([
            'apiBaseUrl' => 'https://api.example.com',
            'apiKey' => $this->credentials['apiKey'],
            'encryptionKey' => ['privateKeyPkcs8B64' => $this->privateKey],
        ], JSON_THROW_ON_ERROR);

        $client = Client::fromCredentials($json);
        self::assertInstanceOf(Client::class, $client);
    }

    /** A real fixtures/credentials.json file path is read from disk and parsed. */
    public function testFromCredentialsFilePath(): void
    {
        $path = Fixtures::dir() . '/credentials.json';
        $client = Client::fromCredentials($path);
        self::assertInstanceOf(Client::class, $client);
    }

    // -- fromCredentials: error paths -----------------------------------------

    public function testFromCredentialsMissingFile(): void
    {
        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('Could not read credentials file');
        Client::fromCredentials('/no/such/credentials.json');
    }

    public function testFromCredentialsMalformedJson(): void
    {
        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('Invalid credentials bundle JSON');
        Client::fromCredentials('{not valid json');
    }

    public function testFromCredentialsMissingApiKey(): void
    {
        $json = json_encode([
            'apiBaseUrl' => 'https://api.example.com',
            'encryptionKey' => ['privateKey' => $this->privateKey],
        ], JSON_THROW_ON_ERROR);

        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('has no apiKey');
        Client::fromCredentials($json);
    }

    public function testFromCredentialsMissingEncryptionKey(): void
    {
        $json = json_encode([
            'apiBaseUrl' => 'https://api.example.com',
            'apiKey' => $this->credentials['apiKey'],
        ], JSON_THROW_ON_ERROR);

        $this->expectException(OpenBankingException::class);
        $this->expectExceptionMessage('has no encryption private key');
        Client::fromCredentials($json);
    }
}
