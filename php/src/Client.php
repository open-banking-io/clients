<?php

declare(strict_types=1);

namespace OpenBankingIO;

use OpenBankingIO\Model\Account;
use OpenBankingIO\Model\Balance;
use OpenBankingIO\Model\Connection;
use OpenBankingIO\Model\SyncAllResult;
use OpenBankingIO\Model\SyncResult;
use OpenBankingIO\Model\Transaction;
use OpenBankingIO\Model\TransactionPage;

/**
 * Server-to-server client for open-banking.io.
 *
 * Authenticates with an API key (X-Api-Key) and decrypts the zero-knowledge data
 * envelopes locally with the exported private key -- the service only ever returns
 * ciphertext it cannot read.
 */
final class Client
{
    private readonly string $apiBaseUrl;
    private readonly string $apiKey;
    private readonly Envelope $envelope;

    public function __construct(string $apiBaseUrl, string $apiKey, string $privateKeyPkcs8)
    {
        if (trim($apiBaseUrl) === '') {
            throw new \InvalidArgumentException('apiBaseUrl is required');
        }
        if (trim($apiKey) === '') {
            throw new \InvalidArgumentException('apiKey is required');
        }
        if (trim($privateKeyPkcs8) === '') {
            throw new \InvalidArgumentException('privateKeyPkcs8 is required');
        }

        $this->apiBaseUrl = rtrim($apiBaseUrl, '/');
        $this->apiKey = $apiKey;
        $this->envelope = Envelope::fromPkcs8Base64($privateKeyPkcs8);
    }

    /**
     * Builds a client from a credentials-bundle JSON string or a path to a bundle file.
     */
    public static function fromCredentials(string $pathOrJson): self
    {
        $raw = $pathOrJson;
        if (str_ends_with(strtolower(trim($pathOrJson)), '.json') || @is_file($pathOrJson)) {
            $contents = @file_get_contents($pathOrJson);
            if ($contents === false) {
                throw new OpenBankingException("Could not read credentials file: {$pathOrJson}");
            }
            $raw = $contents;
        }

        try {
            /** @var array<string, mixed> $bundle */
            $bundle = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        } catch (\JsonException $e) {
            throw new OpenBankingException('Invalid credentials bundle JSON: ' . $e->getMessage());
        }

        $apiBaseUrl = is_string($bundle['apiBaseUrl'] ?? null) ? $bundle['apiBaseUrl'] : '';

        $apiKey = $bundle['apiKey'] ?? null;
        if (!is_string($apiKey) || $apiKey === '') {
            throw new OpenBankingException('The credentials bundle has no apiKey');
        }

        $encKey = is_array($bundle['encryptionKey'] ?? null) ? $bundle['encryptionKey'] : [];
        $privateKey = $encKey['privateKey'] ?? $encKey['privateKeyPkcs8B64'] ?? null;
        if (!is_string($privateKey) || $privateKey === '') {
            throw new OpenBankingException('The credentials bundle has no encryption private key');
        }

        return new self($apiBaseUrl, $apiKey, $privateKey);
    }

    // -- Public API ------------------------------------------------------------

    /**
     * Lists the user's accounts with all sensitive fields decrypted.
     *
     * @return Account[]
     */
    public function getAccounts(): array
    {
        return array_map(
            fn (array $w): Account => $this->mapAccount($w),
            $this->getAccountWires(),
        );
    }

    /**
     * Returns a page of an account's statement, newest first, with decrypted fields.
     *
     * @param array{from?: string, to?: string, limit?: int|string, offset?: int|string} $opts
     */
    public function getTransactions(string $accountId, array $opts = []): TransactionPage
    {
        $query = [];
        foreach (['from', 'to', 'limit', 'offset'] as $key) {
            if (isset($opts[$key]) && $opts[$key] !== '') {
                $query[$key] = (string) $opts[$key];
            }
        }

        $path = 'api/accounts/' . rawurlencode($accountId) . '/transactions';
        if ($query !== []) {
            $path .= '?' . http_build_query($query);
        }

        /** @var array{items?: array<int, array<string, mixed>>, total?: int} $page */
        $page = $this->getJson($path);

        $items = array_map(
            fn (array $t): Transaction => $this->mapTransaction($t),
            $page['items'] ?? [],
        );

        return new TransactionPage($items, (int) ($page['total'] ?? 0));
    }

    /**
     * Lists the user's bank connections.
     *
     * @return Connection[]
     */
    public function getConnections(): array
    {
        /** @var array<int, array<string, mixed>> $rows */
        $rows = $this->getJson('api/connections');

        return array_map(
            fn (array $c): Connection => new Connection(
                sessionId: self::str($c['sessionId'] ?? null),
                aspspName: self::str($c['aspspName'] ?? null),
                aspspCountry: self::str($c['aspspCountry'] ?? null),
                validUntil: self::nullableString($c['validUntil'] ?? null),
                status: self::str($c['status'] ?? null),
                accountCount: self::int($c['accountCount'] ?? null),
                lastSyncedAt: self::nullableString($c['lastSyncedAt'] ?? null),
                psuType: self::nullableString($c['psuType'] ?? null),
            ),
            $rows,
        );
    }

    /**
     * Triggers an online sync of one account.
     *
     * Decrypts that account's Enable Banking uid and posts it, so the service can
     * fetch fresh data without ever holding the uid in plaintext.
     */
    public function sync(string $accountId): SyncResult
    {
        $account = null;
        foreach ($this->getAccountWires() as $wire) {
            if (($wire['id'] ?? null) === $accountId) {
                $account = $wire;
                break;
            }
        }
        if ($account === null) {
            throw new OpenBankingException("Account {$accountId} not found");
        }

        $uid = $this->decryptUid($account);
        if ($uid === null) {
            throw new OpenBankingException(
                'Account has no active session (reconnect required) -- cannot sync',
            );
        }

        $path = 'api/accounts/' . rawurlencode($accountId) . '/sync';
        /** @var array{newTransactions?: int, totalFetched?: int} $result */
        $result = $this->postJson($path, ['uid' => $uid]);

        return new SyncResult(
            newTransactions: (int) ($result['newTransactions'] ?? 0),
            totalFetched: (int) ($result['totalFetched'] ?? 0),
        );
    }

    /**
     * Triggers an online sync of every account that has an active session.
     */
    public function syncAll(): SyncAllResult
    {
        $items = [];
        foreach ($this->getAccountWires() as $wire) {
            $uid = $this->decryptUid($wire);
            if ($uid !== null) {
                $items[] = ['accountId' => $wire['id'] ?? null, 'uid' => $uid];
            }
        }

        /** @var array{accounts?: int, newTransactions?: int} $result */
        $result = $this->postJson('api/sync', ['items' => $items]);

        return new SyncAllResult(
            accounts: (int) ($result['accounts'] ?? 0),
            newTransactions: (int) ($result['newTransactions'] ?? 0),
        );
    }

    // -- Internals -------------------------------------------------------------

    /**
     * @return array<int, array<string, mixed>>
     */
    private function getAccountWires(): array
    {
        /** @var array<int, array<string, mixed>> $wires */
        $wires = $this->getJson('api/accounts');
        return $wires;
    }

    /**
     * @param array<string, mixed> $account
     */
    private function decryptUid(array $account): ?string
    {
        $uidEnc = $account['uidEnc'] ?? null;
        $payload = $this->envelope->decryptToArray(is_string($uidEnc) ? $uidEnc : null);
        $uid = $payload['uid'] ?? null;
        return is_string($uid) ? $uid : null;
    }

    /**
     * @param array<string, mixed> $a
     */
    private function mapAccount(array $a): Account
    {
        $acc = $this->decryptEnc($a['enc'] ?? null);
        $name = $this->decryptEnc($a['displayNameEnc'] ?? null);

        $balances = [];
        $rawBalances = is_array($a['balances'] ?? null) ? $a['balances'] : [];
        foreach ($rawBalances as $b) {
            $b = is_array($b) ? $b : [];
            $dec = $this->decryptEnc($b['enc'] ?? null);
            $balances[] = new Balance(
                type: self::str($b['type'] ?? null),
                name: self::nullableString($dec['name'] ?? null),
                amount: self::decimalString($dec['amount'] ?? null),
                currency: self::str($b['currency'] ?? null),
                referenceDate: self::nullableString($b['referenceDate'] ?? null),
            );
        }

        return new Account(
            id: self::str($a['id'] ?? null),
            aspspName: self::str($a['aspspName'] ?? null),
            aspspCountry: self::str($a['aspspCountry'] ?? null),
            currency: self::str($a['currency'] ?? null),
            accountType: self::nullableString($a['accountType'] ?? null),
            bic: self::nullableString($a['bic'] ?? null),
            needsReconnect: (bool) ($a['needsReconnect'] ?? false),
            iban: self::nullableString($acc['iban'] ?? null),
            bban: self::nullableString($acc['bban'] ?? null),
            ownerName: self::nullableString($acc['ownerName'] ?? null),
            accountName: self::nullableString($acc['accountName'] ?? null),
            product: self::nullableString($acc['product'] ?? null),
            displayName: self::nullableString($name['displayName'] ?? null),
            balances: $balances,
        );
    }

    /**
     * @param array<string, mixed> $t
     */
    private function mapTransaction(array $t): Transaction
    {
        $d = $this->decryptEnc($t['enc'] ?? null);

        return new Transaction(
            id: self::str($t['id'] ?? null),
            currency: self::str($t['currency'] ?? null),
            creditDebitIndicator: self::str($t['creditDebitIndicator'] ?? null),
            status: self::nullableString($t['status'] ?? null),
            bookingDate: self::nullableString($t['bookingDate'] ?? null),
            valueDate: self::nullableString($t['valueDate'] ?? null),
            transactionDate: self::nullableString($t['transactionDate'] ?? null),
            bankTransactionCode: self::nullableString($t['bankTransactionCode'] ?? null),
            amount: self::decimalString($d['amount'] ?? null),
            creditorName: self::nullableString($d['creditorName'] ?? null),
            creditorIban: self::nullableString($d['creditorIban'] ?? null),
            creditorBban: self::nullableString($d['creditorBban'] ?? null),
            creditorAgentBic: self::nullableString($d['creditorAgentBic'] ?? null),
            debtorName: self::nullableString($d['debtorName'] ?? null),
            debtorIban: self::nullableString($d['debtorIban'] ?? null),
            debtorBban: self::nullableString($d['debtorBban'] ?? null),
            debtorAgentBic: self::nullableString($d['debtorAgentBic'] ?? null),
            remittanceInformation: self::nullableString($d['remittanceInformation'] ?? null),
            note: self::nullableString($d['note'] ?? null),
            referenceNumber: self::nullableString($d['referenceNumber'] ?? null),
            exchangeRate: self::nullableString($d['exchangeRate'] ?? null),
            merchantCategoryCode: self::nullableString($d['merchantCategoryCode'] ?? null),
            balanceAfter: self::nullableDecimalString($d['balanceAfter'] ?? null),
            balanceAfterCurrency: self::nullableString($d['balanceAfterCurrency'] ?? null),
            rawJson: self::nullableString($d['rawJson'] ?? null),
        );
    }

    /**
     * @param mixed $enc
     * @return array<string, mixed>
     */
    private function decryptEnc($enc): array
    {
        return $this->envelope->decryptToArray(is_string($enc) ? $enc : null) ?? [];
    }

    // -- HTTP ------------------------------------------------------------------

    /**
     * @return array<int|string, mixed>
     */
    private function getJson(string $path): array
    {
        return $this->request('GET', $path, null);
    }

    /**
     * @param array<string, mixed> $body
     * @return array<int|string, mixed>
     */
    private function postJson(string $path, array $body): array
    {
        return $this->request('POST', $path, $body);
    }

    /**
     * @param 'GET'|'POST' $method
     * @param array<string, mixed>|null $body
     * @return array<int|string, mixed>
     */
    private function request(string $method, string $path, ?array $body): array
    {
        $url = $this->apiBaseUrl . '/' . ltrim($path, '/');

        $headers = ['X-Api-Key: ' . $this->apiKey, 'Accept: application/json'];

        $ch = curl_init();
        curl_setopt($ch, CURLOPT_URL, $url);
        curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
        curl_setopt($ch, CURLOPT_CUSTOMREQUEST, $method);

        if ($body !== null) {
            $payload = json_encode($body, JSON_THROW_ON_ERROR);
            curl_setopt($ch, CURLOPT_POSTFIELDS, $payload);
            $headers[] = 'Content-Type: application/json';
        }

        curl_setopt($ch, CURLOPT_HTTPHEADER, $headers);

        $responseBody = curl_exec($ch);
        if ($responseBody === false) {
            throw new ApiException("HTTP request to {$url} failed: " . curl_error($ch));
        }

        $status = (int) curl_getinfo($ch, CURLINFO_RESPONSE_CODE);

        /** @var string $responseBody */
        if ($status < 200 || $status >= 300) {
            throw new ApiException(
                "{$method} {$path} failed with HTTP {$status}",
                statusCode: $status,
                responseBody: $responseBody,
            );
        }

        try {
            /** @var array<int|string, mixed> $decoded */
            $decoded = json_decode($responseBody, true, 512, JSON_THROW_ON_ERROR);
        } catch (\JsonException $e) {
            throw new ApiException("{$method} {$path} returned invalid JSON: " . $e->getMessage());
        }

        return $decoded;
    }

    // -- Coercion helpers ------------------------------------------------------

    /**
     * Coerces an arbitrary decoded-JSON value to a non-null string ('' default).
     *
     * @param mixed $value
     */
    private static function str($value): string
    {
        return self::nullableString($value) ?? '';
    }

    /**
     * Coerces an arbitrary decoded-JSON value to an int (0 for non-numeric).
     *
     * @param mixed $value
     */
    private static function int($value): int
    {
        return is_numeric($value) ? (int) $value : 0;
    }

    /**
     * Coerces an arbitrary decoded-JSON value to a string, or null for
     * non-scalar / null / empty values. Never throws on arrays or objects.
     *
     * @param mixed $value
     */
    private static function nullableString($value): ?string
    {
        if (!is_scalar($value)) {
            return null;
        }
        $str = is_bool($value) ? ($value ? '1' : '0') : (string) $value;
        return $str === '' ? null : $str;
    }

    /**
     * Amounts are kept as exact decimal strings; never a float.
     *
     * @param mixed $value
     */
    private static function decimalString($value): string
    {
        return self::nullableString($value) ?? '0';
    }

    /**
     * @param mixed $value
     */
    private static function nullableDecimalString($value): ?string
    {
        return self::nullableString($value);
    }
}
