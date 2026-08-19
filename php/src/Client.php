<?php

declare(strict_types=1);

namespace OpenBankingIO;

use OpenBankingIO\Internal\EnvelopeResult;
use OpenBankingIO\Internal\Secret;
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
    /**
     * Client version, sent as the User-Agent. Tracks the published release tag
     * (PHP has no build manifest -- Packagist is tag-based), so bump this when tagging.
     */
    public const VERSION = '1.0.0';

    /** Total request timeout, in seconds. */
    private const TIMEOUT_SECONDS = 30;

    /** Connection-establishment timeout, in seconds. */
    private const CONNECT_TIMEOUT_SECONDS = 10;

    private readonly string $apiBaseUrl;
    private readonly Secret $apiKey;
    private readonly Envelope $envelope;

    /** Effective total request timeout, in seconds (caller may override). */
    private readonly int $timeoutSeconds;

    /** Effective connection-establishment timeout, in seconds (caller may override). */
    private readonly int $connectTimeoutSeconds;

    /**
     * The README recommends curl_options for mTLS, so it can hold CURLOPT_SSLCERTPASSWD or
     * CURLOPT_USERPWD. Kept out of reach for the same reason as the api key: __debugInfo() hides it
     * from var_dump, but VarDumper reads real properties and crash reporters upload what it emits.
     *
     * @var \WeakMap<self, array<int, mixed>>|null
     */
    private static ?\WeakMap $transportOptions = null;

    /**
     * @param array{
     *     curl_options?: array<int, mixed>,
     *     timeout?: int,
     *     connect_timeout?: int,
     * } $options Optional transport overrides. Supported keys:
     *   - `curl_options`: array of `CURLOPT_* => value` merged **last**, so a caller can set
     *     `CURLOPT_PROXY`, `CURLOPT_CAINFO`, mTLS options, etc.
     *   - `timeout`: total request timeout in seconds (overrides the SDK default).
     *   - `connect_timeout`: connection-establishment timeout in seconds (overrides the SDK default).
     */
    public function __construct(
        string $apiBaseUrl,
        #[\SensitiveParameter] string $apiKey,
        #[\SensitiveParameter] string $privateKeyPkcs8,
        #[\SensitiveParameter] array $options = [],
    ) {
        if (trim($apiBaseUrl) === '') {
            throw new OpenBankingException('apiBaseUrl is required');
        }
        if (trim($apiKey) === '') {
            throw new OpenBankingException('apiKey is required');
        }
        if (trim($privateKeyPkcs8) === '') {
            throw new OpenBankingException('privateKeyPkcs8 is required');
        }

        $this->apiBaseUrl = rtrim($apiBaseUrl, '/');
        $this->apiKey = new Secret($apiKey);
        $this->envelope = Envelope::fromPkcs8Base64($privateKeyPkcs8);

        $this->timeoutSeconds = isset($options['timeout']) ? (int) $options['timeout'] : self::TIMEOUT_SECONDS;
        $this->connectTimeoutSeconds = isset($options['connect_timeout'])
            ? (int) $options['connect_timeout']
            : self::CONNECT_TIMEOUT_SECONDS;
        /** @var array<int, mixed> $curlOptions */
        $curlOptions = is_array($options['curl_options'] ?? null) ? $options['curl_options'] : [];
        self::$transportOptions ??= new \WeakMap();
        self::$transportOptions[$this] = $curlOptions;
    }

    /**
     * Builds a client from a credentials-bundle JSON string or a path to a bundle file.
     *
     * @param array{
     *     curl_options?: array<int, mixed>,
     *     timeout?: int,
     *     connect_timeout?: int,
     * } $options Optional transport overrides; see the constructor.
     */
    public static function fromCredentials(#[\SensitiveParameter] string $pathOrJson, #[\SensitiveParameter] array $options = []): self
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

        return new self($apiBaseUrl, $apiKey, $privateKey, $options);
    }

    /**
     * Transport options live in a WeakMap keyed on the instance, and __clone cannot reach the source
     * object to copy them, so a clone would keep authenticating while quietly dropping the proxy, CA
     * bundle or client certificate. Build a second client instead.
     */
    public function __clone(): void
    {
        throw new OpenBankingException('A Client must not be cloned; construct a new one instead');
    }

    /**
     * Covers var_dump and print_r. var_export and an (array) cast read properties directly, which
     * is why the credentials are held out of any property at all.
     *
     * @return array<string, string>
     */
    public function __debugInfo(): array
    {
        return [
            'apiBaseUrl' => $this->apiBaseUrl,
            'apiKey' => '[redacted]',
            'envelope' => '[redacted]',
            'curlOptions' => '[redacted]',
        ];
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

        $page = $this->getJson($path);

        if (!array_key_exists('total', $page) || !array_key_exists('items', $page)) {
            throw new ApiException("Unexpected response from {$path}: expected items and total");
        }

        $items = array_map(
            fn (array $t): Transaction => $this->mapTransaction($t),
            self::rowList($page['items'], $path),
        );

        return new TransactionPage($items, self::requiredInt($page['total'], 'total', $path));
    }

    /**
     * Lists the user's bank connections.
     *
     * @return Connection[]
     */
    public function getConnections(): array
    {
        $rows = self::rowList($this->getJson('api/connections'), 'api/connections');

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
     *
     * By default the service fetches incrementally, from what it already holds. Pass a
     * `fromDate` to backfill from a specific day instead: the import is additive, so this
     * only ever inserts rows that are missing. It is also the lever for a bank that cannot
     * serve a wide window inside one request -- a narrow window returns in seconds where the
     * default may run for minutes and be cut off by a proxy long before it finishes.
     *
     * The window is a request, not a guarantee: read `SyncResult::$servedFromDate` for the one
     * the service actually fetched.
     *
     * @param array{fromDate?: string} $opts `fromDate`: backfill start, YYYY-MM-DD.
     */
    public function sync(string $accountId, array $opts = []): SyncResult
    {
        $fromDate = isset($opts['fromDate']) && $opts['fromDate'] !== ''
            ? self::calendarDate($opts['fromDate'])
            : null;

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

        $session = $this->openEnvelope($account['uidEnc'] ?? null);
        if ($session->error !== null) {
            throw new OpenBankingException(
                "Account {$accountId} session could not be decrypted: {$session->error}",
            );
        }

        $uid = $session->get('uid');
        if (!is_string($uid)) {
            throw new OpenBankingException(
                'Account has no active session (reconnect required) -- cannot sync',
            );
        }

        $body = ['uid' => $uid];
        if ($fromDate !== null) {
            $body['fromDate'] = $fromDate;
        }

        $path = 'api/accounts/' . rawurlencode($accountId) . '/sync';
        $response = $this->postJson($path, $body);
        $result = self::counters($response, ['newTransactions', 'totalFetched'], $path);

        return new SyncResult(
            newTransactions: $result['newTransactions'],
            totalFetched: $result['totalFetched'],
            servedFromDate: self::nullableString($response['servedFromDate'] ?? null),
        );
    }

    /**
     * Triggers an online sync of every account that has an active session.
     */
    public function syncAll(): SyncAllResult
    {
        $items = [];
        $sealed = [];

        foreach ($this->getAccountWires() as $wire) {
            $id = self::str($wire['id'] ?? null);
            $session = $this->openEnvelope($wire['uidEnc'] ?? null);

            if ($session->error !== null) {
                $sealed[self::unreadableKey($sealed, $id)] = $session->error;

                continue;
            }

            if ($session->isAbsent()) {
                // No session at all: the account is simply not connected, which is not a fault.
                continue;
            }

            $uid = $session->get('uid');

            if (is_string($uid)) {
                $items[] = ['accountId' => $id, 'uid' => $uid];

                continue;
            }

            $sealed[self::unreadableKey($sealed, $id)] = 'session envelope carries no usable uid';
        }

        if ($items === [] && $sealed !== []) {
            throw new OpenBankingException(
                'No account session could be used, so nothing was synced: ' . implode('; ', array_map(
                    static fn (string $id, string $reason): string => "{$id}: {$reason}",
                    array_keys($sealed),
                    $sealed,
                )),
            );
        }

        $result = self::counters($this->postJson('api/sync', ['items' => $items]), ['accounts', 'newTransactions'], 'api/sync');

        return new SyncAllResult(
            accounts: $result['accounts'],
            newTransactions: $result['newTransactions'],
            unreadable: $sealed,
        );
    }

    // -- Internals -------------------------------------------------------------

    /**
     * @return array<int, array<string, mixed>>
     */
    private function getAccountWires(): array
    {
        return self::rowList($this->getJson('api/accounts'), 'api/accounts');
    }

    /**
     * @param array<int|string, mixed> $decoded
     * @param array<int, string> $keys
     * @return array<string, int>
     */
    private static function counters(array $decoded, array $keys, string $path): array
    {
        $counters = [];

        foreach ($keys as $key) {
            if (!array_key_exists($key, $decoded)) {
                throw new ApiException("Unexpected response from {$path}: expected {$key}");
            }

            $counters[$key] = self::requiredInt($decoded[$key], $key, $path);
        }

        return $counters;
    }

    /**
     * Refuses anything that is not a plain YYYY-MM-DD calendar day, locally.
     *
     * The service answers a bad date with a 400 whose body does not name the offending argument,
     * and on the sync endpoint that round trip is not free -- it queues behind the account's own
     * work and can cost minutes at a slow bank. `checkdate` is what rejects 2026-02-31, which
     * matches the pattern and is still not a day.
     */
    private static function calendarDate(mixed $value): string
    {
        if (!is_string($value)
            || preg_match('/^(\d{4})-(\d{2})-(\d{2})$/D', $value, $m) !== 1
            || !checkdate((int) $m[2], (int) $m[3], (int) $m[1])) {
            throw new OpenBankingException(
                'fromDate must be a calendar date as YYYY-MM-DD, got: ' . self::describe($value),
            );
        }

        return $value;
    }

    /** Names a rejected argument without ever stringifying an array or an object. */
    private static function describe(mixed $value): string
    {
        return is_string($value) ? $value : get_debug_type($value);
    }

    /**
     * @param mixed $decoded
     * @return array<int, array<string, mixed>>
     */
    private static function rowList($decoded, string $path): array
    {
        if (!is_array($decoded) || ($decoded !== [] && !array_is_list($decoded))) {
            throw new ApiException("Unexpected response from {$path}: expected a list of objects");
        }

        $rows = [];
        foreach ($decoded as $row) {
            if (!is_array($row)) {
                throw new ApiException("Unexpected response from {$path}: expected a list of objects");
            }

            $rows[] = $row;
        }

        /** @var array<int, array<string, mixed>> $rows */
        return $rows;
    }

    /**
     * @param array<string, mixed> $a
     */
    private function mapAccount(array $a): Account
    {
        $acc = $this->openEnvelope($a['enc'] ?? null);
        $name = $this->openEnvelope($a['displayNameEnc'] ?? null);

        $balances = [];
        $rawBalances = is_array($a['balances'] ?? null) ? $a['balances'] : [];
        foreach ($rawBalances as $b) {
            $b = is_array($b) ? $b : [];
            $dec = $this->openEnvelope($b['enc'] ?? null);
            $balances[] = new Balance(
                type: self::str($b['type'] ?? null),
                name: self::nullableString($dec->get('name')),
                amount: self::nullableString($dec->get('amount')),
                currency: self::str($b['currency'] ?? null),
                referenceDate: self::nullableString($b['referenceDate'] ?? null),
                decryptError: $dec->error,
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
            iban: self::nullableString($acc->get('iban')),
            bban: self::nullableString($acc->get('bban')),
            ownerName: self::nullableString($acc->get('ownerName')),
            accountName: self::nullableString($acc->get('accountName')),
            product: self::nullableString($acc->get('product')),
            displayName: self::nullableString($name->get('displayName')),
            balances: $balances,
            decryptError: self::describeErrors([
                'account' => $acc->error,
                'displayName' => $name->error,
                'balances' => self::describeErrors(array_map(
                    static fn (Balance $balance): ?string => $balance->decryptError,
                    $balances,
                )),
            ]),
        );
    }

    /**
     * @param array<string, mixed> $t
     */
    private function mapTransaction(array $t): Transaction
    {
        $d = $this->openEnvelope($t['enc'] ?? null);

        return new Transaction(
            id: self::str($t['id'] ?? null),
            currency: self::str($t['currency'] ?? null),
            creditDebitIndicator: self::str($t['creditDebitIndicator'] ?? null),
            status: self::nullableString($t['status'] ?? null),
            bookingDate: self::nullableString($t['bookingDate'] ?? null),
            valueDate: self::nullableString($t['valueDate'] ?? null),
            transactionDate: self::nullableString($t['transactionDate'] ?? null),
            bankTransactionCode: self::nullableString($t['bankTransactionCode'] ?? null),
            amount: self::nullableString($d->get('amount')),
            creditorName: self::nullableString($d->get('creditorName')),
            creditorIban: self::nullableString($d->get('creditorIban')),
            creditorBban: self::nullableString($d->get('creditorBban')),
            creditorAgentBic: self::nullableString($d->get('creditorAgentBic')),
            debtorName: self::nullableString($d->get('debtorName')),
            debtorIban: self::nullableString($d->get('debtorIban')),
            debtorBban: self::nullableString($d->get('debtorBban')),
            debtorAgentBic: self::nullableString($d->get('debtorAgentBic')),
            remittanceInformation: self::nullableString($d->get('remittanceInformation')),
            note: self::nullableString($d->get('note')),
            referenceNumber: self::nullableString($d->get('referenceNumber')),
            exchangeRate: self::nullableString($d->get('exchangeRate')),
            merchantCategoryCode: self::nullableString($d->get('merchantCategoryCode')),
            balanceAfter: self::nullableString($d->get('balanceAfter')),
            balanceAfterCurrency: self::nullableString($d->get('balanceAfterCurrency')),
            rawJson: self::nullableString($d->get('rawJson')),
            decryptError: $d->error,
        );
    }

    /**
     * Names which envelope failed, so a display-name failure is not mistaken for the
     * account's own fields being unreadable.
     *
     * @param array<array-key, string|null> $errors
     */
    private static function describeErrors(array $errors): ?string
    {
        $described = [];

        foreach ($errors as $envelope => $error) {
            if ($error !== null) {
                $described[] = "{$envelope}: {$error}";
            }
        }

        return $described === [] ? null : implode('; ', $described);
    }

    /**
     * @param mixed $enc
     */
    private function openEnvelope($enc): EnvelopeResult
    {
        if ($enc === null || $enc === '') {
            return EnvelopeResult::absent();
        }

        if (!is_string($enc)) {
            return EnvelopeResult::sealed('Envelope field is ' . get_debug_type($enc) . ', not a string');
        }

        try {
            return EnvelopeResult::opened($this->envelope->decryptToArray($enc) ?? []);
        } catch (EnvelopeException | \JsonException $e) {
            return EnvelopeResult::sealed($e->getMessage());
        }
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

        $headers = ['X-Api-Key: ' . $this->apiKey->reveal(), 'Accept: application/json'];

        $ch = curl_init();
        curl_setopt($ch, CURLOPT_URL, $url);
        curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
        curl_setopt($ch, CURLOPT_CUSTOMREQUEST, $method);
        curl_setopt($ch, CURLOPT_USERAGENT, 'open-banking-io/php/' . self::VERSION);
        curl_setopt($ch, CURLOPT_TIMEOUT, $this->timeoutSeconds);
        curl_setopt($ch, CURLOPT_CONNECTTIMEOUT, $this->connectTimeoutSeconds);

        if ($body !== null) {
            $payload = json_encode($body, JSON_THROW_ON_ERROR);
            curl_setopt($ch, CURLOPT_POSTFIELDS, $payload);
            $headers[] = 'Content-Type: application/json';
        }

        $curlOptions = self::$transportOptions[$this] ?? [];
        $curlOptions = is_array($curlOptions) ? $curlOptions : [];

        // curl_setopt_array replaces an array option rather than merging it, so a caller adding one
        // header would drop X-Api-Key and authenticate as nobody.
        $callerHeaders = $curlOptions[CURLOPT_HTTPHEADER] ?? null;

        if (is_array($callerHeaders)) {
            foreach ($callerHeaders as $header) {
                if (is_string($header)) {
                    $headers[] = $header;
                }
            }

            unset($curlOptions[CURLOPT_HTTPHEADER]);
        }

        curl_setopt($ch, CURLOPT_HTTPHEADER, $headers);

        // Caller-supplied cURL options are applied LAST so they win over the SDK defaults
        // (proxy, custom CA/CAINFO, mTLS client cert, keep-alive, etc.).
        if ($curlOptions !== []) {
            try {
                curl_setopt_array($ch, $curlOptions);
            } catch (\ValueError $e) {
                throw new OpenBankingException('A curl_options entry was rejected: ' . $e->getMessage());
            }
        }

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
            $decoded = json_decode($responseBody, true, 512, JSON_THROW_ON_ERROR);
        } catch (\JsonException $e) {
            throw new ApiException("{$method} {$path} returned invalid JSON: " . $e->getMessage());
        }

        // A captive portal or a gateway can answer 200 with a bare scalar. Returning it would
        // trip this method's own return type, and a TypeError is not what callers catch.
        if (!is_array($decoded)) {
            throw new ApiException(
                "{$method} {$path} returned " . get_debug_type($decoded) . ', expected an object or a list',
            );
        }

        /** @var array<int|string, mixed> $decoded */
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
     * An account the caller can act on, and never one that silently replaces another: a provider
     * repeating an id, or omitting it, must not make the unreadable set under-report.
     *
     * @param array<string, string> $taken
     */
    private static function unreadableKey(array $taken, string $id): string
    {
        $base = $id === '' ? '(account with no id)' : $id;

        if (!array_key_exists($base, $taken)) {
            return $base;
        }

        for ($n = 2; array_key_exists("{$base} (#{$n})", $taken); $n++) {
            // find the first free suffix
        }

        return "{$base} (#{$n})";
    }

    /**
     * A required counter, validated rather than coerced: a malformed total or newTransactions
     * would otherwise read as a finished empty page or a successful no-op.
     *
     * @param mixed $value
     */
    private static function requiredInt($value, string $key, string $path): int
    {
        // /D anchors $ at the very end: without it PCRE also matches before a trailing newline, so
        // "5\n" would pass a check whose whole job is to validate rather than coerce.
        if (!is_int($value) && !(is_string($value) && preg_match('/^-?\d+$/D', $value) === 1)) {
            throw new ApiException(
                "Unexpected response from {$path}: {$key} is " . get_debug_type($value) . ', expected an integer',
            );
        }

        // A digit string past the integer range saturates silently, which would report a plausible
        // row count for a response that cannot be right. Equal-length digit strings compare the
        // same lexicographically as numerically, so this needs no arbitrary-precision arithmetic.
        // The negative range reaches one further than the positive one.
        if (is_string($value)) {
            $negative = str_starts_with($value, '-');
            $digits = ltrim(ltrim($value, '-'), '0');
            $ceiling = $negative ? ltrim((string) PHP_INT_MIN, '-') : (string) PHP_INT_MAX;

            if (strlen($digits) > strlen($ceiling) || (strlen($digits) === strlen($ceiling) && $digits > $ceiling)) {
                throw new ApiException("Unexpected response from {$path}: {$key} is out of range");
            }
        }

        return (int) $value;
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

}
