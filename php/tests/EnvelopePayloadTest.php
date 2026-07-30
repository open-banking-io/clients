<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

use OpenBankingIO\Envelope;
use OpenBankingIO\EnvelopeException;
use PHPUnit\Framework\Attributes\DataProvider;
use PHPUnit\Framework\TestCase;

/**
 * A decrypted envelope is trusted to be a JSON object. Anything else must surface as an
 * envelope failure the caller can skip, never as a TypeError that aborts a whole page.
 */
final class EnvelopePayloadTest extends TestCase
{
    private Envelope $envelope;

    private string $recipientPublicPem;

    protected function setUp(): void
    {
        /** @var array{privateKeyPkcs8B64: string} $keypair */
        $keypair = Fixtures::load('keypair.json');

        $this->envelope = Envelope::fromPkcs8Base64($keypair['privateKeyPkcs8B64']);

        $private = openssl_pkey_get_private(
            "-----BEGIN PRIVATE KEY-----\n" // gitleaks:allow
            . chunk_split($keypair['privateKeyPkcs8B64'], 64, "\n")
            . "-----END PRIVATE KEY-----\n",
        );
        self::assertNotFalse($private);

        $details = openssl_pkey_get_details($private);
        self::assertIsArray($details);
        self::assertIsString($details['key']);
        $this->recipientPublicPem = $details['key'];
    }

    public function testAJsonObjectPayloadDecryptsToAnArray(): void
    {
        $payload = $this->envelope->decryptToArray($this->seal('{"amount":"9.95"}'));

        self::assertSame(['amount' => '9.95'], $payload);
    }

    /**
     * @return array<int, array{0: string}>
     */
    public static function scalarPayloads(): array
    {
        // '[]' is deliberately absent: an empty JSON object decodes to the same empty PHP array
        // as an empty list, so the two cannot be told apart and the empty case is allowed.
        return [['5'], ['"just a string"'], ['null'], ['true'], ['1.5'], ['[1,2,3]'], ['["a"]']];
    }

    #[DataProvider('scalarPayloads')]
    public function testAScalarPayloadIsRejectedAsAnEnvelopeFailure(string $json): void
    {
        $this->expectException(EnvelopeException::class);
        $this->expectExceptionMessage('not a JSON object');

        $this->envelope->decryptToArray($this->seal($json));
    }

    private function seal(string $plaintext): string
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
