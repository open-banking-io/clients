<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

use OpenBankingIO\Envelope;
use OpenBankingIO\EnvelopeException;
use PHPUnit\Framework\TestCase;

/**
 * Negative-path coverage for the zero-knowledge envelope: malformed wire frames,
 * invalid keys, and tampered points must all raise EnvelopeException rather than
 * silently returning garbage. Reuses the shared fixtures.
 */
final class EnvelopeNegativeTest extends TestCase
{
    private const VERSION = 0x01;
    private const POINT_LEN = 65;
    private const NONCE_LEN = 12;
    private const TAG_LEN = 16;
    private const HEADER_LEN = 1 + self::POINT_LEN + self::NONCE_LEN + self::TAG_LEN; // 94

    private Envelope $envelope;

    protected function setUp(): void
    {
        /** @var array{privateKeyPkcs8B64: string} $keypair */
        $keypair = Fixtures::load('keypair.json');
        $this->envelope = Envelope::fromPkcs8Base64($keypair['privateKeyPkcs8B64']);
    }

    /** A frame shorter than the fixed header must be rejected outright. */
    public function testTruncatedEnvelopeTooShort(): void
    {
        $raw = "\x01" . str_repeat("\x00", self::HEADER_LEN - 2); // one byte short of header

        $this->expectException(EnvelopeException::class);
        $this->expectExceptionMessage('Invalid or unsupported envelope');
        $this->envelope->decrypt($raw);
    }

    /** An empty raw frame is also too short. */
    public function testEmptyRawEnvelopeRejected(): void
    {
        $this->expectException(EnvelopeException::class);
        $this->expectExceptionMessage('Invalid or unsupported envelope');
        $this->envelope->decrypt('');
    }

    /** A full-length frame carrying an unsupported version byte must be rejected. */
    public function testWrongVersionByte(): void
    {
        // Header length is satisfied, but the version byte is 0x02 (not 0x01).
        $raw = "\x02" . str_repeat("\x00", self::HEADER_LEN); // header + 1 byte ciphertext

        $this->expectException(EnvelopeException::class);
        $this->expectExceptionMessage('Invalid or unsupported envelope');
        $this->envelope->decrypt($raw);
    }

    /** A correctly sized frame whose ephemeral point is not a 0x04-prefixed P-256 point. */
    public function testGarbageEphemeralPublicKeyPointPrefix(): void
    {
        // Version OK and length OK, but the ephemeral point starts with 0x05, not 0x04.
        $point = "\x05" . str_repeat("\x00", self::POINT_LEN - 1);
        $rest = str_repeat("\x00", self::NONCE_LEN + self::TAG_LEN + 4);
        $raw = chr(self::VERSION) . $point . $rest;

        $this->expectException(EnvelopeException::class);
        $this->expectExceptionMessage('Invalid ephemeral public key point');
        $this->envelope->decrypt($raw);
    }

    /**
     * A 0x04-prefixed but otherwise random point is not a valid curve point;
     * OpenSSL must reject the key (or the subsequent ECDH must fail).
     */
    public function testGarbageEphemeralPublicKeyOffCurve(): void
    {
        $point = "\x04" . random_bytes(self::POINT_LEN - 1);
        $rest = str_repeat("\x00", self::NONCE_LEN + self::TAG_LEN + 4);
        $raw = chr(self::VERSION) . $point . $rest;

        $this->expectException(EnvelopeException::class);
        $this->envelope->decrypt($raw);
    }

    /** Non-base64 input to decryptToArray must raise rather than decode to junk. */
    public function testInvalidBase64Envelope(): void
    {
        $this->expectException(EnvelopeException::class);
        $this->expectExceptionMessage('Invalid base64 envelope');
        $this->envelope->decryptToArray('not valid base64 !!!@@@');
    }

    /** Garbage that is not a PKCS#8 key at all. */
    public function testInvalidPkcs8KeyGarbage(): void
    {
        $this->expectException(EnvelopeException::class);
        $this->expectExceptionMessage('Invalid PKCS#8 private key');
        Envelope::fromPkcs8Base64('bm90LWEta2V5'); // base64("not-a-key")
    }

    /** A structurally valid PKCS#8 key of the wrong type (RSA, not EC) is rejected. */
    public function testNonEcPkcs8KeyRejected(): void
    {
        $rsa = openssl_pkey_new([
            'private_key_type' => OPENSSL_KEYTYPE_RSA,
            'private_key_bits' => 2048,
        ]);
        self::assertNotFalse($rsa);
        $pem = '';
        openssl_pkey_export($rsa, $pem);
        self::assertIsString($pem);
        $b64 = preg_replace('/-----[^-]+-----|\s+/', '', $pem) ?? '';

        $this->expectException(EnvelopeException::class);
        $this->expectExceptionMessage('not an EC key');
        Envelope::fromPkcs8Base64($b64);
    }
}
