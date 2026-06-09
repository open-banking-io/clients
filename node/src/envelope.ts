import { webcrypto } from "node:crypto";

const { subtle } = webcrypto;

/** Re-export the WebCrypto key type so callers don't depend on the DOM lib. */
export type CryptoKey = webcrypto.CryptoKey;

const VERSION = 0x01;
const POINT_LEN = 65;
const NONCE_LEN = 12;
const TAG_LEN = 16;

const HKDF_INFO = new TextEncoder().encode("bank.core.ci/zk/v1");
const HKDF_SALT = new Uint8Array(32); // 32 zero bytes (must be exactly this).

/**
 * Decrypts open-banking.io's zero-knowledge data envelopes.
 * Scheme: ephemeral ECDH on NIST P-256 -> HKDF-SHA256 -> AES-256-GCM.
 * Wire: `version(1)=0x01 | ephemeralPublicKeyRaw(65) | nonce(12) | tag(16) | ciphertext`.
 * Only the user's private key can decrypt — the service stores ciphertext it cannot read.
 */
export async function decryptEnvelope(
  privateKey: CryptoKey,
  envelopeBase64: string,
): Promise<Uint8Array> {
  const bytes = base64ToBytes(envelopeBase64);
  if (bytes.length < 1 + POINT_LEN + NONCE_LEN + TAG_LEN || bytes[0] !== VERSION) {
    throw new Error("Invalid or unsupported envelope");
  }

  const ephPub = bytes.subarray(1, 1 + POINT_LEN);
  const nonce = bytes.subarray(1 + POINT_LEN, 1 + POINT_LEN + NONCE_LEN);
  const tag = bytes.subarray(1 + POINT_LEN + NONCE_LEN, 1 + POINT_LEN + NONCE_LEN + TAG_LEN);
  const ciphertext = bytes.subarray(1 + POINT_LEN + NONCE_LEN + TAG_LEN);

  const ephKey = await subtle.importKey(
    "raw",
    ephPub,
    { name: "ECDH", namedCurve: "P-256" },
    false,
    [],
  );

  const shared = new Uint8Array(
    await subtle.deriveBits({ name: "ECDH", public: ephKey }, privateKey, 256),
  );

  const hkdf = await subtle.importKey("raw", shared, "HKDF", false, ["deriveKey"]);

  const aesKey = await subtle.deriveKey(
    { name: "HKDF", hash: "SHA-256", salt: HKDF_SALT, info: HKDF_INFO },
    hkdf,
    { name: "AES-GCM", length: 256 },
    false,
    ["decrypt"],
  );

  // Web Crypto AES-GCM expects ciphertext||tag.
  const ctWithTag = new Uint8Array(ciphertext.length + tag.length);
  ctWithTag.set(ciphertext, 0);
  ctWithTag.set(tag, ciphertext.length);

  const plaintext = await subtle.decrypt(
    { name: "AES-GCM", iv: nonce, tagLength: 128 },
    aesKey,
    ctWithTag,
  );

  return new Uint8Array(plaintext);
}

/** Decrypts a base64 envelope and JSON-parses its payload. `null`/`undefined` in -> `null`. */
export async function decryptTo<T>(
  privateKey: CryptoKey,
  envelopeBase64: string | null | undefined,
): Promise<T | null> {
  if (envelopeBase64 == null) return null;
  const plaintext = await decryptEnvelope(privateKey, envelopeBase64);
  return JSON.parse(new TextDecoder().decode(plaintext)) as T;
}

/** Imports a base64 PKCS#8 P-256 ECDH private key for use with {@link decryptEnvelope}. */
export async function importPrivateKey(pkcs8Base64: string): Promise<CryptoKey> {
  return subtle.importKey(
    "pkcs8",
    base64ToBytes(pkcs8Base64),
    { name: "ECDH", namedCurve: "P-256" },
    false,
    ["deriveBits"],
  );
}

function base64ToBytes(b64: string): Uint8Array {
  return new Uint8Array(Buffer.from(b64, "base64"));
}
