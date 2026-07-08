'use strict';

// Zero-knowledge envelope decryptor for open-banking.io.
//
// This is a self-contained port of the @open-banking-io/client `envelope.ts`.
// It deliberately uses ZERO runtime dependencies and relies only on the
// Web Crypto API exposed as the `crypto.subtle` global (present in Node 20+,
// which is Zapier's runtime).
//
// Vendored here (rather than imported from the SDK) so the Zapier app
// has no external npm dependencies for the crypto layer.

const subtle = crypto.subtle;

const VERSION = 0x01;
const POINT_LEN = 65;
const NONCE_LEN = 12;
const TAG_LEN = 16;

const HKDF_INFO = new TextEncoder().encode('bank.core.ci/zk/v1');
const HKDF_SALT = new Uint8Array(32); // 32 zero bytes (must be exactly this).

/**
 * Decrypts open-banking.io's zero-knowledge data envelopes.
 * Scheme: ephemeral ECDH on NIST P-256 -> HKDF-SHA256 -> AES-256-GCM.
 * Wire: `version(1)=0x01 | ephemeralPublicKeyRaw(65) | nonce(12) | tag(16) | ciphertext`.
 * Only the user's private key can decrypt — the service stores ciphertext it cannot read.
 *
 * @param {CryptoKey} privateKey — imported P-256 ECDH private key
 * @param {string} envelopeBase64 — base64-encoded envelope
 * @returns {Promise<Uint8Array>} decrypted plaintext bytes
 */
async function decryptEnvelope(privateKey, envelopeBase64) {
  const bytes = base64ToBytes(envelopeBase64);
  if (bytes.length < 1 + POINT_LEN + NONCE_LEN + TAG_LEN || bytes[0] !== VERSION) {
    throw new Error('Invalid or unsupported envelope');
  }

  const ephPub = bytes.subarray(1, 1 + POINT_LEN);
  const nonce = bytes.subarray(1 + POINT_LEN, 1 + POINT_LEN + NONCE_LEN);
  const tag = bytes.subarray(1 + POINT_LEN + NONCE_LEN, 1 + POINT_LEN + NONCE_LEN + TAG_LEN);
  const ciphertext = bytes.subarray(1 + POINT_LEN + NONCE_LEN + TAG_LEN);

  // Guard against invalid-curve attacks: WebCrypto's P-256 `importKey` validates that the raw point
  // lies on the curve and is not the point at infinity, so a forged/off-curve ephemeral public key
  // throws here rather than yielding a derivable (and exploitable) shared secret below.
  const ephKey = await subtle.importKey(
    'raw',
    ephPub,
    { name: 'ECDH', namedCurve: 'P-256' },
    false,
    [],
  );

  const shared = new Uint8Array(
    await subtle.deriveBits({ name: 'ECDH', public: ephKey }, privateKey, 256),
  );

  const hkdf = await subtle.importKey('raw', shared, 'HKDF', false, ['deriveKey']);

  const aesKey = await subtle.deriveKey(
    { name: 'HKDF', hash: 'SHA-256', salt: HKDF_SALT, info: HKDF_INFO },
    hkdf,
    { name: 'AES-GCM', length: 256 },
    false,
    ['decrypt'],
  );

  // Web Crypto AES-GCM expects ciphertext||tag.
  const ctWithTag = new Uint8Array(ciphertext.length + tag.length);
  ctWithTag.set(ciphertext, 0);
  ctWithTag.set(tag, ciphertext.length);

  const plaintext = await subtle.decrypt(
    { name: 'AES-GCM', iv: nonce, tagLength: 128 },
    aesKey,
    ctWithTag,
  );

  return new Uint8Array(plaintext);
}

/**
 * Decrypts a base64 envelope and JSON-parses its payload.
 * @param {CryptoKey} privateKey
 * @param {string | null | undefined} envelopeBase64
 * @returns {Promise<any | null>} parsed JSON or null if input is null/undefined
 */
async function decryptTo(privateKey, envelopeBase64) {
  if (envelopeBase64 == null) return null;
  const plaintext = await decryptEnvelope(privateKey, envelopeBase64);
  return JSON.parse(new TextDecoder().decode(plaintext));
}

/**
 * Imports a base64 PKCS#8 P-256 ECDH private key for use with decryptEnvelope.
 * @param {string} pkcs8Base64
 * @returns {Promise<CryptoKey>}
 */
async function importPrivateKey(pkcs8Base64) {
  return subtle.importKey(
    'pkcs8',
    base64ToBytes(pkcs8Base64),
    { name: 'ECDH', namedCurve: 'P-256' },
    false,
    ['deriveBits'],
  );
}

/**
 * Buffer-free base64 -> bytes. `atob` is a global in Node 16+ and Zapier's runtime.
 *
 * @param {string} b64
 * @returns {Uint8Array}
 */
function base64ToBytes(b64) {
  const binary = atob(b64);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    out[i] = binary.charCodeAt(i);
  }
  return out;
}

module.exports = { decryptEnvelope, decryptTo, importPrivateKey };
