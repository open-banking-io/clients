<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

use OpenBankingIO\ApiException;
use OpenBankingIO\Client;
use OpenBankingIO\Envelope;
use OpenBankingIO\EnvelopeException;
use PHPUnit\Framework\TestCase;
use Symfony\Component\VarDumper\Cloner\VarCloner;
use Symfony\Component\VarDumper\Dumper\CliDumper;

/**
 * These behaviours are invisible to the happy path, so without this file CI stays green if any
 * of them is dropped.
 */
final class HardeningTest extends TestCase
{
    use SealsEnvelopes;

    public function testRejectsAKeyOnTheWrongCurve(): void
    {
        $key = openssl_pkey_new(['private_key_type' => OPENSSL_KEYTYPE_EC, 'curve_name' => 'secp384r1']);
        self::assertNotFalse($key);

        openssl_pkey_export($key, $pem);
        self::assertIsString($pem);

        $this->expectException(EnvelopeException::class);
        $this->expectExceptionMessage('not a P-256 key');

        Envelope::fromPkcs8Base64($this->derBase64($pem));
    }

    public function testRejectsAKeyStillWearingItsPemArmor(): void
    {
        $keypair = Fixtures::load('keypair.json');

        $key = $keypair['privateKeyPkcs8B64'];
        self::assertIsString($key);

        $armored = "-----BEGIN PRIVATE KEY-----\n" // gitleaks:allow
            . chunk_split($key, 64, "\n")
            . "-----END PRIVATE KEY-----\n";

        $this->expectException(EnvelopeException::class);
        $this->expectExceptionMessage('bare base64');

        Envelope::fromPkcs8Base64($armored);
    }

    public function testKeyMaterialDoesNotSurviveIntoAStackTrace(): void
    {
        $secret = 'not-a-real-key-but-distinctive';

        try {
            Envelope::fromPkcs8Base64($secret);
            self::fail('expected the key to be rejected');
        } catch (EnvelopeException $e) {
            self::assertStringNotContainsString($secret, print_r($e->getTrace(), true));
        }
    }

    public function testTheApiKeyIsNotPrintedWhenTheClientIsDumped(): void
    {
        $keypair = Fixtures::load('keypair.json');

        $key = $keypair['privateKeyPkcs8B64'];
        self::assertIsString($key);

        $client = new Client('https://example.test', 'ebk_super_secret', $key);

        self::assertStringNotContainsString('ebk_super_secret', print_r($client, true));
    }

    public function testARejectedKeyLeavesNothingInTheOpenSslErrorQueue(): void
    {
        while (openssl_error_string() !== false) {
            // start from a clean queue
        }

        try {
            // A bad GCM tag happens to push nothing on some OpenSSL builds; a key OpenSSL cannot
            // decode always does, which is what makes this assertion load-bearing.
            Envelope::fromPkcs8Base64('bm90LWEta2V5');
        } catch (EnvelopeException) {
            // the failure is the point
        }

        self::assertFalse(openssl_error_string(), 'a later unrelated openssl call would report this key');
    }

    /**
     * Laravel's dd()/dump() and every Ignition or Flare style crash reporter are built on
     * VarDumper, which emits real properties alongside __debugInfo() and expands a closure's
     * captured variables and an OpenSSLAsymmetricKey's EC components. It is the vector that
     * matters most and the one a redacting logger cannot help with, because the value does not
     * appear under a field called apiKey.
     */
    public function testNeitherCredentialSurvivesADumperBasedCrashReport(): void
    {
        $keypair = Fixtures::load('keypair.json');
        $key = $keypair['privateKeyPkcs8B64'];
        self::assertIsString($key);

        // The README recommends curl_options for mTLS, so a client-certificate passphrase and
        // proxy credentials are realistic values to find in there.
        $client = new Client('https://example.test', 'obk_live_distinctive_key', $key, [
            'curl_options' => [
                CURLOPT_SSLCERTPASSWD => 'mtls-passphrase-distinctive',
                CURLOPT_USERPWD => 'proxyuser:proxypass-distinctive',
            ],
        ]);

        $dumped = (new CliDumper())->dump((new VarCloner())->cloneVar($client), true);
        self::assertIsString($dumped);

        self::assertStringNotContainsString('obk_live_distinctive_key', $dumped);
        self::assertStringNotContainsString('mtls-passphrase-distinctive', $dumped);
        self::assertStringNotContainsString('proxypass-distinctive', $dumped);
        self::assertDoesNotMatchRegularExpression('/"d" =>/', $dumped, 'the P-256 private scalar must not be dumped');
    }

    public function testTheApiKeyIsNotPrintedByVarExportOrAnArrayCast(): void
    {
        $keypair = Fixtures::load('keypair.json');
        $key = $keypair['privateKeyPkcs8B64'];
        self::assertIsString($key);

        $client = new Client('https://example.test', 'ebk_super_secret', $key);

        self::assertStringNotContainsString('ebk_super_secret', var_export($client, true));
        self::assertStringNotContainsString('ebk_super_secret', print_r((array) $client, true));
    }

    /**
     * The invariant is that no envelope operation leaves anything behind for the next openssl call
     * in the same worker. On this build a bad GCM tag happens to push nothing, so removing
     * decrypt()'s finally is not observable here -- it is insurance for OpenSSL 1.1.1/3.0, where
     * EVP_DecryptFinal_ex does push. The key-loading path below is what pins the drains that are
     * observable everywhere.
     */
    public function testNoEnvelopeOperationLeavesAnythingInTheOpenSslErrorQueue(): void
    {
        $keypair = Fixtures::load('keypair.json');
        $key = $keypair['privateKeyPkcs8B64'];
        self::assertIsString($key);

        $this->useRecipientKey($key);
        $envelope = Envelope::fromPkcs8Base64($key);
        $sealed = $this->seal('{"amount":"9.95"}');

        foreach (['a successful decrypt', 'a tampered envelope', 'an off-curve ephemeral point'] as $i => $case) {
            while (openssl_error_string() !== false) {
                // start each case from a clean queue
            }

            try {
                match ($i) {
                    0 => $envelope->decryptToArray($sealed),
                    1 => $envelope->decryptToArray(substr($sealed, 0, -4) . 'AAAA'),
                    default => $envelope->decrypt("\x01\x04" . str_repeat("\xAA", 64) . str_repeat("\x00", 28) . 'ct'),
                };
            } catch (EnvelopeException | \JsonException) {
                // the failure is the point
            }

            self::assertFalse(openssl_error_string(), "queue dirty after {$case}");
        }
    }

    public function testRejectingAKeyEarlyDoesNotPassSomebodyElsesErrorsOn(): void
    {
        // Something unrelated dirtied the queue first. The armor check returns before any OpenSSL
        // call, so only the drain on entry stops those entries being read as this envelope's.
        openssl_pkey_get_private('not a key at all');
        self::assertNotFalse(openssl_error_string(), 'expected the queue to be dirty to begin with');
        openssl_pkey_get_private('not a key at all');

        try {
            Envelope::fromPkcs8Base64("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----"); // gitleaks:allow
            self::fail('expected the armored key to be rejected');
        } catch (EnvelopeException $e) {
            self::assertStringContainsString('bare base64', $e->getMessage());
        }

        self::assertFalse(openssl_error_string());
    }

    public function testSerialisingAClientFailsLoudlyRatherThanSilentlyEmptying(): void
    {
        $keypair = Fixtures::load('keypair.json');
        $key = $keypair['privateKeyPkcs8B64'];
        self::assertIsString($key);

        $client = new Client('https://example.test', 'obk_live_x', $key);

        // Frozen on purpose: a queued Client would otherwise come back holding nothing, and the
        // first request would fail somewhere far from the cause.
        $this->expectException(\OpenBankingIO\OpenBankingException::class);

        serialize($client);
    }

    public function testTheRedactingHooksThemselvesAreStillInPlace(): void
    {
        $keypair = Fixtures::load('keypair.json');
        $key = $keypair['privateKeyPkcs8B64'];
        self::assertIsString($key);

        $client = new Client('https://example.test', 'obk_live_x', $key);
        $secret = new \OpenBankingIO\Internal\Secret('obk_live_y');

        // The WeakMap is what keeps the value unreachable, but these hooks are what a reader sees
        // instead of nothing at all, so they should not quietly disappear.
        self::assertSame('[redacted]', $client->__debugInfo()['apiKey']);
        self::assertSame('[redacted]', $client->__debugInfo()['curlOptions']);
        self::assertSame('[redacted]', $secret->__debugInfo()['value']);
        self::assertSame('[redacted]', (string) $secret);
    }

    public function testTransportCredentialsDoNotSurviveAThrowingConstructor(): void
    {
        $options = ['curl_options' => [
            CURLOPT_SSLCERTPASSWD => 'mtls-passphrase-distinctive',
            CURLOPT_USERPWD => 'proxyuser:proxypass-distinctive',
        ]];

        // The WeakMap closes the property surface. A throwing constructor is a separate surface:
        // the whole options array sits in the stack frame unless it is marked sensitive too.
        foreach ([['https://example.test', 'k', 'not-a-key'], ['', 'k', 'k'], ['https://example.test', '', 'k']] as $args) {
            try {
                new Client($args[0], $args[1], $args[2], $options);
                self::fail('expected the constructor to reject these arguments');
            } catch (\Throwable $e) {
                $trace = print_r($e->getTrace(), true);
                self::assertStringNotContainsString('mtls-passphrase-distinctive', $trace);
                self::assertStringNotContainsString('proxypass-distinctive', $trace);
                self::assertStringNotContainsString('mtls-passphrase-distinctive', print_r($e, true));
            }
        }
    }

    public function testTransportCredentialsDoNotSurviveAThrowingFactory(): void
    {
        $options = ['curl_options' => [CURLOPT_SSLCERTPASSWD => 'factory-passphrase-distinctive']];

        foreach (['{"not":"a bundle"}', 'not json at all', '{"apiKey":"k"}'] as $bundle) {
            try {
                Client::fromCredentials($bundle, $options);
                self::fail('expected the bundle to be rejected');
            } catch (\Throwable $e) {
                self::assertStringNotContainsString('factory-passphrase-distinctive', print_r($e->getTrace(), true));
                self::assertStringNotContainsString('factory-passphrase-distinctive', print_r($e, true));
            }
        }
    }

    public function testTheIntegerRangeCheckUsesTheRightCeilingForEachSign(): void
    {
        $requiredInt = new \ReflectionMethod(Client::class, 'requiredInt');

        // The negative range reaches one further than the positive one, and a total is the only
        // caller, so nothing else would ever exercise this boundary.
        self::assertSame(PHP_INT_MIN, $requiredInt->invoke(null, (string) PHP_INT_MIN, 'total', '/p'));
        self::assertSame(PHP_INT_MAX, $requiredInt->invoke(null, (string) PHP_INT_MAX, 'total', '/p'));

        foreach (['-9223372036854775809', '9223372036854775808'] as $beyond) {
            try {
                $requiredInt->invoke(null, $beyond, 'total', '/p');
                self::fail("{$beyond} should be out of range");
            } catch (\ReflectionException | ApiException $e) {
                self::assertStringContainsString('out of range', $e->getMessage());
            }
        }
    }

    public function testAClientRefusesToBeClonedRatherThanLosingItsTransportOptions(): void
    {
        $keypair = Fixtures::load('keypair.json');
        $key = $keypair['privateKeyPkcs8B64'];
        self::assertIsString($key);

        $client = new Client('https://example.test', 'k', $key, ['curl_options' => [CURLOPT_USERAGENT => 'ua']]);

        // A clone gets no WeakMap entry, so it would keep authenticating while silently dropping the
        // proxy, CA bundle or client certificate.
        $this->expectException(\OpenBankingIO\OpenBankingException::class);

        $clone = clone $client;
    }

    private function derBase64(string $pem): string
    {
        $body = preg_replace('#-----[A-Z ]+-----#', '', $pem);

        return str_replace(["\r", "\n"], '', (string) $body);
    }
}
