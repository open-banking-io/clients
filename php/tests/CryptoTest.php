<?php

declare(strict_types=1);

namespace OpenBankingIO\Tests;

use OpenBankingIO\Envelope;
use OpenBankingIO\EnvelopeException;
use PHPUnit\Framework\TestCase;

/** Wire-format interop: decrypt the shared fixtures and assert they match expected. */
final class CryptoTest extends TestCase
{
    private Envelope $envelope;
    /** @var array<string, string> */
    private array $envelopes;
    /** @var array<string, mixed> */
    private array $expected;

    protected function setUp(): void
    {
        /** @var array{privateKeyPkcs8B64: string} $keypair */
        $keypair = Fixtures::load('keypair.json');
        $this->envelope = Envelope::fromPkcs8Base64($keypair['privateKeyPkcs8B64']);
        /** @var array<string, string> $envelopes */
        $envelopes = Fixtures::load('envelopes.json');
        $this->envelopes = $envelopes;
        /** @var array<string, mixed> $expected */
        $expected = Fixtures::load('expected.json');
        $this->expected = $expected;
    }

    public function testAccountEnvelope(): void
    {
        $acc = $this->envelope->decryptToArray($this->envelopes['account']);
        self::assertSame($this->expected['account'], $acc);
        self::assertNotNull($acc);
        self::assertSame('DK6466952001724927', $acc['iban']);
    }

    public function testDisplayNameEnvelope(): void
    {
        $name = $this->envelope->decryptToArray($this->envelopes['displayName']);
        self::assertSame($this->expected['displayName'], $name);
        self::assertNotNull($name);
        self::assertSame('Drift', $name['displayName']);
    }

    public function testUidEnvelope(): void
    {
        $uid = $this->envelope->decryptToArray($this->envelopes['uid']);
        self::assertNotNull($uid);
        self::assertSame('c5d93aa7-5e23-4da0-ba88-42b9a584492c', $uid['uid']);
        /** @var array{uid: string} $expectedUid */
        $expectedUid = $this->expected['uid'];
        self::assertSame($expectedUid['uid'], $uid['uid']);
    }

    public function testBalanceEnvelope(): void
    {
        // The shared balance envelope is the booked (ITBD) balance.
        $bal = $this->envelope->decryptToArray($this->envelopes['balance']);
        self::assertNotNull($bal);
        self::assertSame('828.13', $bal['amount']);
        /** @var array{ITBD: array{amount: string}} $expectedBalances */
        $expectedBalances = $this->expected['balances'];
        self::assertSame($expectedBalances['ITBD']['amount'], $bal['amount']);
        self::assertSame('Tatic', $bal['name']);
    }

    public function testTransactionEnvelope(): void
    {
        $txn = $this->envelope->decryptToArray($this->envelopes['transaction']);
        self::assertNotNull($txn);
        /** @var array{amount: string} $exp */
        $exp = $this->expected['transaction'];
        self::assertSame('194.23', $txn['amount']);
        self::assertSame($exp['amount'], $txn['amount']);
        self::assertSame('One.com', $txn['creditorName']);
        self::assertSame('4816', $txn['merchantCategoryCode']);
        self::assertSame('633.90', $txn['balanceAfter']);
    }

    public function testNullEnvelopeYieldsNull(): void
    {
        self::assertNull($this->envelope->decryptToArray(null));
        self::assertNull($this->envelope->decryptToArray(''));
    }

    public function testWrongKeyFailsToDecrypt(): void
    {
        // A structurally valid but different P-256 key must fail the GCM auth tag.
        $other = openssl_pkey_new([
            'private_key_type' => OPENSSL_KEYTYPE_EC,
            'curve_name' => 'prime256v1',
        ]);
        self::assertNotFalse($other);
        $pem = '';
        openssl_pkey_export($other, $pem);
        self::assertIsString($pem);
        // Strip the PEM armor back to base64 DER for fromPkcs8Base64.
        $b64 = preg_replace('/-----[^-]+-----|\s+/', '', $pem) ?? '';

        $wrong = Envelope::fromPkcs8Base64($b64);
        $this->expectException(EnvelopeException::class);
        $wrong->decryptToArray($this->envelopes['account']);
    }
}
