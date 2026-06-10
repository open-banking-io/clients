<?php

declare(strict_types=1);

/**
 * Router for the PHP built-in server used as a mock open-banking.io API in tests.
 *
 * Serves the shared fixtures/api/*.json, enforces the X-Api-Key header (401 otherwise),
 * and records POST request bodies to OBK_CAPTURE_FILE so the test can assert on them.
 *
 * Env: OBK_FIXTURES (path to repo fixtures/), OBK_API_KEY, OBK_CAPTURE_FILE.
 */

$fixtures = getenv('OBK_FIXTURES') ?: '';
$apiKey = getenv('OBK_API_KEY') ?: '';
$captureFile = getenv('OBK_CAPTURE_FILE') ?: '';

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
    $serve('accounts.json');
    return true;
}

if ($method === 'GET' && $path === "/api/accounts/{$accountId}/transactions") {
    $serve('transactions.json');
    return true;
}

if ($method === 'GET' && $path === '/api/connections') {
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
