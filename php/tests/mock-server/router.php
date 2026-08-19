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
 *   'bad-json'     -> GET /api/connections returns 200 with an unparseable body,
 *   'scalar-body'  -> GET /api/accounts returns 200 with a bare JSON scalar,
 *   'object-body'  -> GET /api/accounts returns 200 with an object where a list is expected,
 *   'keyed-body'   -> GET /api/accounts returns 200 with a keyed map of rows,
 *   'scalar-rows'  -> GET /api/accounts returns a list whose entries are not objects,
 *   'bad-total'    -> the transactions page returns a non-integer total,
 *   'sealed-session' -> the /api/accounts account carries a uidEnc nobody can decrypt,
 *   'mixed-fleet'  -> one account readable, one sealed, one with no session at all,
 *   'uidless-session' -> the account's session envelope opens but carries no uid,
 *   'huge-total'   -> the transactions page returns a total past PHP_INT_MAX,
 *   'no-total'     -> the transactions page omits total, 'no-items' omits items,
 *   'newline-total' -> total is a digit string with a trailing newline,
 *   'edge-total'    -> total is exactly PHP_INT_MAX, 'over-edge-total' is one past it,
 *   'twin-sealed'  -> two accounts share an id and both have unreadable sessions,
 *   'sealed-display-name' -> only the account's displayNameEnc is unreadable,
 *   'sync-no-counters' -> POST /api/sync returns 200 without its counters,
 *   'sync-narrowed-window' -> the single-account sync answers with a servedFromDate
 *                             narrower than any window a test would ask for.
 */

$fixtures = getenv('OBK_FIXTURES') ?: '';
$apiKey = getenv('OBK_API_KEY') ?: '';
$captureFile = getenv('OBK_CAPTURE_FILE') ?: '';
$accountsMode = getenv('OBK_ACCOUNTS_MODE') ?: '';

// OBK_DELAY_MS delays every response by N milliseconds, so a test can drive the
// client's request timeout against a deliberately slow server.
$delayMs = (int) (getenv('OBK_DELAY_MS') ?: '0');
if ($delayMs > 0) {
    usleep($delayMs * 1000);
}

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
        $existing['apiKeyHeader'] = is_string($_SERVER['HTTP_X_API_KEY'] ?? null)
            ? $_SERVER['HTTP_X_API_KEY']
            : '';
        $existing['callerHeader'] = is_string($_SERVER['HTTP_X_CALLER_TAG'] ?? null)
            ? $_SERVER['HTTP_X_CALLER_TAG']
            : '';
        file_put_contents($captureFile, json_encode($existing));
    }
    if ($accountsMode === 'scalar-body') {
        echo '42';
        return true;
    }
    if ($accountsMode === 'object-body') {
        echo json_encode(['error' => 'rate limited']);
        return true;
    }
    if ($accountsMode === 'scalar-rows') {
        echo json_encode(['11111111-1111-4111-8111-111111111111', 'another']);
        return true;
    }
    if ($accountsMode === 'keyed-body') {
        echo json_encode(['unexpected' => ['id' => '11111111-1111-4111-8111-111111111111']]);
        return true;
    }
    if ($accountsMode === 'uidless-session') {
        $raw = file_get_contents($fixtures . '/api/accounts.json');
        $decoded = $raw === false ? [] : json_decode($raw, true);
        $accounts = is_array($decoded) ? $decoded : [];
        $uidless = getenv('OBK_UIDLESS_ENC') ?: '';
        foreach ($accounts as &$acct) {
            if (is_array($acct)) {
                $acct['uidEnc'] = $uidless;
            }
        }
        unset($acct);
        echo json_encode($accounts);
        return true;
    }
    if ($accountsMode === 'sealed-display-name') {
        $raw = file_get_contents($fixtures . '/api/accounts.json');
        $decoded = $raw === false ? [] : json_decode($raw, true);
        $accounts = is_array($decoded) ? $decoded : [];
        foreach ($accounts as &$acct) {
            if (is_array($acct)) {
                $acct['displayNameEnc'] = base64_encode("\x01" . str_repeat("\x00", 92) . 'nonsense');
            }
        }
        unset($acct);
        echo json_encode($accounts);
        return true;
    }
    if ($accountsMode === 'twin-sealed') {
        $raw = file_get_contents($fixtures . '/api/accounts.json');
        $decoded = $raw === false ? [] : json_decode($raw, true);
        $accounts = is_array($decoded) ? $decoded : [];
        $first = $accounts[0] ?? [];
        $row = is_array($first) ? $first : [];
        $row['uidEnc'] = base64_encode("\x01" . str_repeat("\x00", 92) . 'nonsense');

        $twin = $row;
        unset($twin['id']);

        echo json_encode([$row, $row, $twin, $twin]);
        return true;
    }
    if ($accountsMode === 'mixed-fleet') {
        $raw = file_get_contents($fixtures . '/api/accounts.json');
        $decoded = $raw === false ? [] : json_decode($raw, true);
        $accounts = is_array($decoded) ? $decoded : [];
        $first = $accounts[0] ?? [];
        $readable = is_array($first) ? $first : [];

        $torn = $readable;
        $torn['id'] = '22222222-2222-4222-8222-222222222222';
        $torn['uidEnc'] = base64_encode("\x01" . str_repeat("\x00", 92) . 'nonsense');

        $unconnected = $readable;
        $unconnected['id'] = '33333333-3333-4333-8333-333333333333';
        unset($unconnected['uidEnc']);

        echo json_encode([$readable, $torn, $unconnected]);
        return true;
    }
    if ($accountsMode === 'sealed-session') {
        $raw = file_get_contents($fixtures . '/api/accounts.json');
        $decoded = $raw === false ? [] : json_decode($raw, true);
        $accounts = is_array($decoded) ? $decoded : [];
        foreach ($accounts as &$acct) {
            if (is_array($acct)) {
                $acct['uidEnc'] = base64_encode("\x01" . str_repeat("\x00", 92) . 'nonsense');
            }
        }
        unset($acct);
        echo json_encode($accounts);
        return true;
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
    if ($accountsMode === 'bad-total') {
        echo json_encode(['items' => [], 'total' => []]);
        return true;
    }
    if ($accountsMode === 'no-total') {
        echo json_encode(['items' => []]);
        return true;
    }
    if ($accountsMode === 'no-items') {
        echo json_encode(['total' => 0]);
        return true;
    }
    if ($accountsMode === 'newline-total') {
        echo '{"items":[],"total":"5\\n"}';
        return true;
    }
    if ($accountsMode === 'edge-total') {
        echo '{"items":[],"total":"9223372036854775807"}';
        return true;
    }
    if ($accountsMode === 'over-edge-total') {
        echo '{"items":[],"total":"9223372036854775808"}';
        return true;
    }
    if ($accountsMode === 'huge-total') {
        echo '{"items":[],"total":"99999999999999999999999999"}';
        return true;
    }
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
    if ($accountsMode === 'sync-narrowed-window') {
        echo json_encode(['newTransactions' => 0, 'totalFetched' => 1, 'servedFromDate' => '2026-05-19']);
        return true;
    }
    $serve('sync.json');
    return true;
}

if ($method === 'POST' && $path === '/api/sync') {
    if ($accountsMode === 'sync-no-counters') {
        echo json_encode(['ok' => true]);
        return true;
    }
    $capture('syncAll');
    $serve('sync-all.json');
    return true;
}

http_response_code(404);
echo json_encode(['error' => 'not found', 'path' => $path]);
return true;
