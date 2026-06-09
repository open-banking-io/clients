// Generates the shared, language-agnostic test fixtures for all three SDKs.
//
// It mirrors the open-banking.io backend's zero-knowledge envelope format EXACTLY
// (ECDH P-256 -> HKDF-SHA256 -> AES-256-GCM), so every SDK that can decrypt these
// fixtures is proven to interoperate with the real service.
//
// Run: node tools/generate-fixtures.mjs   (writes ./fixtures/**)
//
// Note: crypto uses randomness, so re-running produces new (equally valid) fixtures.
// The SDKs verify by decrypting to the committed `expected.json`, not by byte-equality.

import { webcrypto as crypto } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const FIX = join(ROOT, 'fixtures');
const te = (s) => new TextEncoder().encode(s);
const b64 = (u8) => Buffer.from(u8).toString('base64');
const INFO = te('bank.core.ci/zk/v1');

/** Encrypt a JSON-serialisable object to a recipient's raw P-256 public key. */
async function encryptEnvelope(recipientPubRaw, obj) {
  const recipient = await crypto.subtle.importKey(
    'raw', recipientPubRaw, { name: 'ECDH', namedCurve: 'P-256' }, false, []);
  const eph = await crypto.subtle.generateKey({ name: 'ECDH', namedCurve: 'P-256' }, true, ['deriveBits']);
  const shared = new Uint8Array(await crypto.subtle.deriveBits({ name: 'ECDH', public: recipient }, eph.privateKey, 256));

  const hkdf = await crypto.subtle.importKey('raw', shared, 'HKDF', false, ['deriveKey']);
  const aesKey = await crypto.subtle.deriveKey(
    { name: 'HKDF', hash: 'SHA-256', salt: new Uint8Array(32), info: INFO },
    hkdf, { name: 'AES-GCM', length: 256 }, false, ['encrypt']);

  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const ctTag = new Uint8Array(await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: nonce, tagLength: 128 }, aesKey, te(JSON.stringify(obj))));
  const ct = ctTag.slice(0, ctTag.length - 16);
  const tag = ctTag.slice(ctTag.length - 16);
  const ephRaw = new Uint8Array(await crypto.subtle.exportKey('raw', eph.publicKey)); // 65 bytes

  // wire: version(1) | ephPub(65) | nonce(12) | tag(16) | ciphertext
  const env = new Uint8Array(1 + 65 + 12 + 16 + ct.length);
  let o = 0;
  env[o++] = 1;
  env.set(ephRaw, o); o += 65;
  env.set(nonce, o); o += 12;
  env.set(tag, o); o += 16;
  env.set(ct, o);
  return b64(env);
}

const main = async () => {
  // --- test keypair ---
  const kp = await crypto.subtle.generateKey({ name: 'ECDH', namedCurve: 'P-256' }, true, ['deriveBits']);
  const privatePkcs8 = b64(new Uint8Array(await crypto.subtle.exportKey('pkcs8', kp.privateKey)));
  const publicRaw = new Uint8Array(await crypto.subtle.exportKey('raw', kp.publicKey));
  const publicRawB64 = b64(publicRaw);

  // --- plaintext payloads (the exact field contracts the backend encrypts) ---
  const expected = {
    account: { ownerName: 'Tatic ApS', iban: 'DK6466952001724927', bban: '66952001724927', accountName: 'Erhverv', product: 'Business' },
    displayName: { displayName: 'Drift' },
    uid: { uid: 'c5d93aa7-5e23-4da0-ba88-42b9a584492c' },
    balances: {
      ITBD: { amount: '828.13', name: 'Tatic' },
      ITAV: { amount: '633.90', name: 'Tatic' },
      OTHR: { amount: '194.23', name: 'Tatic' },
    },
    transaction: {
      amount: '194.23', creditorName: 'One.com', creditorIban: null, creditorBban: null, creditorAgentBic: null,
      debtorName: null, debtorIban: null, debtorBban: null, debtorAgentBic: null,
      remittanceInformation: 'One.com', note: null, referenceNumber: null, exchangeRate: null,
      merchantCategoryCode: '4816', balanceAfter: '633.90', balanceAfterCurrency: 'DKK', rawJson: '{"id":"txn-1"}',
    },
  };

  // --- single representative envelopes (for the crypto unit test) ---
  const envelopes = {
    account: await encryptEnvelope(publicRaw, expected.account),
    displayName: await encryptEnvelope(publicRaw, expected.displayName),
    uid: await encryptEnvelope(publicRaw, expected.uid),
    balance: await encryptEnvelope(publicRaw, expected.balances.ITBD),
    transaction: await encryptEnvelope(publicRaw, expected.transaction),
  };

  // --- credentials bundle (what the app exports) ---
  const credentials = {
    service: 'open-banking.io',
    apiBaseUrl: 'http://localhost:8081',
    user: 'demo@open-banking.io',
    exportedAt: '2026-06-09T16:00:00.000Z',
    apiKey: 'obk_test_3f8b9c2e1a7d4655b0e9f2c1a8d7e6f5',
    encryptionKey: {
      scheme: 'ECDH-P256 + HKDF-SHA256 + AES-256-GCM',
      curve: 'P-256',
      privateKeyFormat: 'pkcs8-base64',
      privateKey: privatePkcs8,
      publicKey: publicRawB64,
      envelope: {
        layout: 'version(1) | ephemeralPublicKeyRaw(65) | nonce(12) | tag(16) | ciphertext(N)',
        hkdf: { hash: 'SHA-256', saltHex: '00'.repeat(32), info: 'bank.core.ci/zk/v1' },
        aead: 'AES-256-GCM (96-bit nonce, 128-bit tag); ciphertext||tag for Web Crypto',
      },
    },
  };

  // --- encrypted API responses (what the SDK fetches over HTTP) ---
  const accountId = '11111111-1111-4111-8111-111111111111';
  const sessionId = '22222222-2222-4222-8222-222222222222';

  const accountsResponse = [{
    id: accountId,
    iban: null, bban: null, ownerName: null, accountName: null, displayName: null, product: null,
    aspspName: 'Lunar', aspspCountry: 'DK', currency: 'DKK', accountType: 'CACC', bic: 'LUNADK22',
    needsReconnect: false,
    balances: [
      { type: 'ITBD', name: null, amount: 0, currency: 'DKK', referenceDate: null, enc: await encryptEnvelope(publicRaw, expected.balances.ITBD) },
      { type: 'ITAV', name: null, amount: 0, currency: 'DKK', referenceDate: null, enc: await encryptEnvelope(publicRaw, expected.balances.ITAV) },
      { type: 'OTHR', name: null, amount: 0, currency: 'DKK', referenceDate: null, enc: await encryptEnvelope(publicRaw, expected.balances.OTHR) },
    ],
    enc: await encryptEnvelope(publicRaw, expected.account),
    displayNameEnc: await encryptEnvelope(publicRaw, expected.displayName),
    uidEnc: await encryptEnvelope(publicRaw, expected.uid),
  }];

  const transactionsResponse = {
    items: [{
      id: '33333333-3333-4333-8333-333333333333',
      amount: 0, currency: 'DKK', creditDebitIndicator: 'DBIT', status: 'BOOK',
      bookingDate: '2026-06-08', valueDate: null, transactionDate: '2026-06-08',
      creditorName: null, creditorIban: null, debtorName: null, debtorIban: null,
      remittanceInformation: null, bankTransactionCode: null,
      balanceAfterTransaction: null, balanceAfterCurrency: null,
      merchantCategoryCode: null, creditorAgentBic: null, debtorAgentBic: null,
      referenceNumber: null, exchangeRate: null, note: null,
      enc: await encryptEnvelope(publicRaw, expected.transaction),
    }],
    total: 1,
  };

  const connectionsResponse = [{
    sessionId, aspspName: 'Lunar', aspspCountry: 'DK',
    validUntil: '2026-09-07T16:00:00.000Z', status: 'Active', accountCount: 1,
    lastSyncedAt: '2026-06-09T16:00:00.000Z', psuType: 'business',
  }];

  // --- write everything ---
  mkdirSync(join(FIX, 'api'), { recursive: true });
  const w = (p, o) => writeFileSync(join(FIX, p), JSON.stringify(o, null, 2) + '\n');
  w('keypair.json', { privateKeyPkcs8B64: privatePkcs8, publicKeyRawB64: publicRawB64 });
  w('credentials.json', credentials);
  w('envelopes.json', envelopes);
  w('expected.json', { ...expected, accountId, uid: expected.uid });
  w('api/accounts.json', accountsResponse);
  w('api/transactions.json', transactionsResponse);
  w('api/connections.json', connectionsResponse);
  w('api/sync.json', { newTransactions: 0, totalFetched: 1 });
  w('api/sync-all.json', { accounts: 1, newTransactions: 0 });

  console.log('Fixtures written to', FIX);
};

main().catch((e) => { console.error(e); process.exit(1); });
