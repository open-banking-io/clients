import { createHash, webcrypto } from "node:crypto";
import { decryptTo, importPrivateKey } from "./envelope.js";

const subtle = webcrypto.subtle;

/** A partner's recipient (decryption) key pair. The private half is a credential: never log it. */
export interface RecipientKeyPair {
  /** Base64 PKCS#8 — what your deployment loads. */
  privateKeyPkcs8Base64: string;
  /** Base64 raw 65-byte uncompressed P-256 point — what you install on your partner page. */
  publicKeyRawBase64: string;
}

/**
 * Generates the P-256 pair a partner installs on open-banking.io. The public half goes into
 * `PUT /api/partner/recipient-key` (or the partner page's paste box); keep the private half
 * where your deployment can read it — the service never sees it and cannot recover it.
 */
export async function generateRecipientKeyPair(): Promise<RecipientKeyPair> {
  const pair = await subtle.generateKey({ name: "ECDH", namedCurve: "P-256" }, true, ["deriveBits"]);
  return {
    privateKeyPkcs8Base64: Buffer.from(await subtle.exportKey("pkcs8", pair.privateKey)).toString("base64"),
    publicKeyRawBase64: Buffer.from(await subtle.exportKey("raw", pair.publicKey)).toString("base64"),
  };
}

/**
 * The fingerprint the partner page and the admin show for a key: SHA-256 of the raw 65-byte
 * point, lowercase hex, first 16 characters. Print it at boot and compare after every deploy.
 */
export function recipientKeyFingerprint(publicKeyRawBase64: string): string {
  return createHash("sha256").update(Buffer.from(publicKeyRawBase64, "base64")).digest("hex").slice(0, 16);
}

/** The raw public point of a PKCS#8 private key — to fingerprint or re-install the key you hold. */
export async function recipientPublicKey(privateKeyPkcs8Base64: string): Promise<string> {
  const key = await subtle.importKey(
    "pkcs8",
    Buffer.from(privateKeyPkcs8Base64, "base64"),
    { name: "ECDH", namedCurve: "P-256" },
    true,
    ["deriveBits"],
  );
  const jwk = await subtle.exportKey("jwk", key);
  if (!jwk.x || !jwk.y) throw new Error("The private key carries no public point");
  return Buffer.concat([Buffer.from([0x04]), Buffer.from(jwk.x, "base64url"), Buffer.from(jwk.y, "base64url")]).toString(
    "base64",
  );
}

/**
 * Answers the possession challenge from `POST /api/partner/recipient-key/challenge`: opens
 * the envelope with the private half and returns the nonce — the `challengeAnswer` for the install.
 */
export async function answerRecipientKeyChallenge(
  privateKeyPkcs8Base64: string,
  envelopeBase64: string,
): Promise<string> {
  const payload = await decryptTo<{ nonce?: unknown }>(await importPrivateKey(privateKeyPkcs8Base64), envelopeBase64);
  const nonce = payload?.nonce;
  if (typeof nonce !== "string" || nonce.length === 0) throw new Error("The challenge envelope carries no nonce");
  return nonce;
}
