<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

/**
 * Seals a payload the way the service does, so a test can build an envelope the client will open
 * rather than asserting against a fixture that cannot be varied.
 */
trait SealsEnvelopes
{
    private string $recipientPublicPem = '';

    protected function useRecipientKey(string $privateKeyPkcs8B64): void
    {
        $private = openssl_pkey_get_private(
            "-----BEGIN PRIVATE KEY-----\n" // gitleaks:allow
            . chunk_split($privateKeyPkcs8B64, 64, "\n")
            . "-----END PRIVATE KEY-----\n",
        );
        self::assertNotFalse($private);

        $details = openssl_pkey_get_details($private);
        self::assertIsArray($details);
        self::assertIsString($details['key']);

        $this->recipientPublicPem = $details['key'];
    }

    protected function seal(string $plaintext): string
    {
        $ephemeral = openssl_pkey_new([
            'private_key_type' => OPENSSL_KEYTYPE_EC,
            'curve_name' => 'prime256v1',
        ]);
        self::assertNotFalse($ephemeral);

        $recipient = openssl_pkey_get_public($this->recipientPublicPem);
        self::assertNotFalse($recipient);

        $shared = openssl_pkey_derive($recipient, $ephemeral);
        self::assertIsString($shared);

        $key = hash_hkdf('sha256', $shared, 32, 'bank.core.ci/zk/v1', str_repeat("\x00", 32));

        $nonce = random_bytes(12);
        $tag = '';
        $ciphertext = openssl_encrypt($plaintext, 'aes-256-gcm', $key, OPENSSL_RAW_DATA, $nonce, $tag, '', 16);
        self::assertIsString($ciphertext);

        $details = openssl_pkey_get_details($ephemeral);
        self::assertIsArray($details);
        self::assertIsArray($details['ec']);
        self::assertIsString($details['ec']['x']);
        self::assertIsString($details['ec']['y']);

        $point = "\x04"
            . str_pad($details['ec']['x'], 32, "\x00", STR_PAD_LEFT)
            . str_pad($details['ec']['y'], 32, "\x00", STR_PAD_LEFT);

        return base64_encode("\x01" . $point . $nonce . $tag . $ciphertext);
    }
}
