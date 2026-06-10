<?php

declare(strict_types=1);

/**
 * Router for the PHP built-in server used as a mock open-banking.io API in tests.
 *
 * Serves the shared fixtures/api/*.json, enforces the X-Api-Key header (401 otherwise),
 * and records POST request bodies to OBK_CAPTURE_FILE so the test can assert on them.
 *
 * Env: OBK_FIXTURES (path to repo fixtures/), OBK_API_KEY, OBK_CAPTURE_FILE.
 *
 * OBK_ACCOUNTS_MODE switches payloads for negative-path tests:
 *   'sessionless'  -> the /api/accounts account drops its uidEnc (no active session),
 *   'error-status' -> GET /api/connections returns 503 with a JSON error body,
 *   'bad-json'     -> GET /api/connections returns 200 with an unparseable body.
 */

$fixtures = getenv('OBK_FIXTURES') ?: '';
$apiKey = getenv('OBK_API_KEY') ?: '';
$captureFile = getenv('OBK_CAPTURE_FILE') ?: '';
$accountsMode = getenv('OBK_ACCOUNTS_MODE') ?: '';

$accountId = '11111111-1111-4111-8111-111111111111';

$requestUri = is_string($_SERVER['REQUEST_URI'] ?? null) ? $_SERVER['REQUEST_URI'] : '/';
$path = parse_url($requestUri, PHP_URL_PATH) ?: '/';
$method = is_string($_SERVER['REQUEST_METHOD'] ?? null) ? $_SERVER['REQUEST_METHOD'] : 'GET';

header('Content-Type: application/json');

$sentKey = $_SERVER['HTTP_X_API_KEY'] ?? '';
if ($sentKey !== $apiKey) {
    http_response_code(401);
    echo json_encode(['error' => 'unauthorized']);
    return true;
}

$serve = static function (string $file) use ($fixtures): void {
    $body = file_get_contents($fixtures . '/api/' . $file);
    if ($body === false) {
        http_response_code(500);
        echo json_encode(['error' => 'fixture missing']);
        return;
    }
    echo $body;
};

$capture = static function (string $label) use ($captureFile): void {
    if ($captureFile === '') {
        return;
    }
    $raw = file_get_contents('php://input') ?: '';
    $existing = [];
    if (is_file($captureFile)) {
        $decoded = json_decode((string) file_get_contents($captureFile), true);
        $existing = is_array($decoded) ? $decoded : [];
    }
    $existing[$label] = json_decode($raw, true);
    file_put_contents($captureFile, json_encode($existing));
};

if ($method === 'GET' && $path === '/api/accounts') {
    if ($captureFile !== '') {
        $existing = [];
        if (is_file($captureFile)) {
            $decoded = json_decode((string) file_get_contents($captureFile), true);
            $existing = is_array($decoded) ? $decoded : [];
        }
        $existing['userAgent'] = is_string($_SERVER['HTTP_USER_AGENT'] ?? null)
            ? $_SERVER['HTTP_USER_AGENT']
            : '';
        file_put_contents($captureFile, json_encode($existing));
    }
    if ($accountsMode === 'sessionless') {
        $raw = file_get_contents($fixtures . '/api/accounts.json');
        $decoded = $raw === false ? [] : json_decode($raw, true);
        $accounts = is_array($decoded) ? $decoded : [];
        foreach ($accounts as &$acct) {
            if (is_array($acct)) {
                unset($acct['uidEnc']);
            }
        }
        unset($acct);
        echo json_encode($accounts);
        return true;
    }
    $serve('accounts.json');
    return true;
}

if ($method === 'GET' && $path === "/api/accounts/{$accountId}/transactions") {
    $serve('transactions.json');
    return true;
}

if ($method === 'GET' && $path === '/api/connections') {
    if ($accountsMode === 'error-status') {
        http_response_code(503);
        echo json_encode(['error' => 'service unavailable']);
        return true;
    }
    if ($accountsMode === 'bad-json') {
        echo 'this is not json {';
        return true;
    }
    $serve('connections.json');
    return true;
}

if ($method === 'POST' && $path === "/api/accounts/{$accountId}/sync") {
    $capture('sync');
    $serve('sync.json');
    return true;
}

if ($method === 'POST' && $path === '/api/sync') {
    $capture('syncAll');
    $serve('sync-all.json');
    return true;
}

http_response_code(404);
echo json_encode(['error' => 'not found', 'path' => $path]);
return true;
