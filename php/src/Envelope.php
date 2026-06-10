<?php

declare(strict_types=1);

namespace OpenBankingIO;

/**
 * Decrypts open-banking.io's zero-knowledge data envelopes.
 *
 * Scheme: ephemeral ECDH on NIST P-256 -> HKDF-SHA256 -> AES-256-GCM.
 * Wire: version(1)=0x01 | ephemeralPublicKeyRaw(65) | nonce(12) | tag(16) | ciphertext.
 * Only the user's private key can decrypt -- the service stores ciphertext it cannot read.
 */
final class Envelope
{
    private const VERSION = 0x01;
    private const POINT_LEN = 65;
    private const NONCE_LEN = 12;
    private const TAG_LEN = 16;
    private const HEADER_LEN = 1 + self::POINT_LEN + self::NONCE_LEN + self::TAG_LEN;

    private const HKDF_INFO = 'bank.core.ci/zk/v1';

    /**
     * Fixed P-256 SubjectPublicKeyInfo DER prefix (26 bytes). The raw 65-byte point
     * (which itself begins with 0x04) is appended to build a parsable SPKI structure.
     */
    private const SPKI_PREFIX_HEX = '3059301306072a8648ce3d020106082a8648ce3d030107034200';

    /** @var \OpenSSLAsymmetricKey */
    private $privateKey;

    private function __construct(\OpenSSLAsymmetricKey $privateKey)
    {
        $this->privateKey = $privateKey;
    }

    /**
     * Loads a base64 PKCS#8 EC (P-256) private key by wrapping the DER in PEM.
     */
    public static function fromPkcs8Base64(string $privateKeyPkcs8B64): self
    {
        $pem = "-----BEGIN PRIVATE KEY-----\n"
            . chunk_split(trim($privateKeyPkcs8B64), 64, "\n")
            . "-----END PRIVATE KEY-----\n";

        $key = openssl_pkey_get_private($pem);
        if ($key === false) {
            throw new EnvelopeException('Invalid PKCS#8 private key');
        }

        $details = openssl_pkey_get_details($key);
        if ($details === false || ($details['type'] ?? -1) !== OPENSSL_KEYTYPE_EC) {
            throw new EnvelopeException('Private key is not an EC key');
        }

        return new self($key);
    }

    /**
     * Decrypts a base64 envelope and parses its JSON payload. A null/empty input yields null.
     *
     * @return array<string, mixed>|null
     */
    public function decryptToArray(?string $envelopeB64): ?array
    {
        if ($envelopeB64 === null || $envelopeB64 === '') {
            return null;
        }

        $raw = base64_decode($envelopeB64, true);
        if ($raw === false) {
            throw new EnvelopeException('Invalid base64 envelope');
        }

        $plaintext = $this->decrypt($raw);

        /** @var array<string, mixed> $payload */
        $payload = json_decode($plaintext, true, 512, JSON_THROW_ON_ERROR);
        return $payload;
    }

    /**
     * Decrypts the raw bytes of a zero-knowledge envelope.
     */
    public function decrypt(string $envelope): string
    {
        if (strlen($envelope) < self::HEADER_LEN || ord($envelope[0]) !== self::VERSION) {
            throw new EnvelopeException('Invalid or unsupported envelope');
        }

        $ephPubRaw = substr($envelope, 1, self::POINT_LEN);
        $nonce = substr($envelope, 1 + self::POINT_LEN, self::NONCE_LEN);
        $tag = substr($envelope, 1 + self::POINT_LEN + self::NONCE_LEN, self::TAG_LEN);
        $ciphertext = substr($envelope, self::HEADER_LEN);

        $ephPub = self::publicKeyFromRawPoint($ephPubRaw);

        $shared = openssl_pkey_derive($ephPub, $this->privateKey);
        if ($shared === false) {
            throw new EnvelopeException('ECDH key agreement failed');
        }

        $key = hash_hkdf('sha256', $shared, 32, self::HKDF_INFO, str_repeat("\x00", 32));

        $plaintext = openssl_decrypt(
            $ciphertext,
            'aes-256-gcm',
            $key,
            OPENSSL_RAW_DATA,
            $nonce,
            $tag,
            '',
        );
        if ($plaintext === false) {
            throw new EnvelopeException('AES-256-GCM decryption failed (wrong key or tampered data)');
        }

        return $plaintext;
    }

    /**
     * Builds an EC public key from a raw 65-byte (0x04 || X || Y) P-256 point.
     */
    private static function publicKeyFromRawPoint(string $rawPoint): \OpenSSLAsymmetricKey
    {
        if (strlen($rawPoint) !== self::POINT_LEN || ord($rawPoint[0]) !== 0x04) {
            throw new EnvelopeException('Invalid ephemeral public key point');
        }

        $der = hex2bin(self::SPKI_PREFIX_HEX) . $rawPoint;
        $pem = "-----BEGIN PUBLIC KEY-----\n"
            . chunk_split(base64_encode($der), 64, "\n")
            . "-----END PUBLIC KEY-----\n";

        $key = openssl_pkey_get_public($pem);
        if ($key === false) {
            throw new EnvelopeException('Invalid ephemeral public key');
        }

        return $key;
    }
}
