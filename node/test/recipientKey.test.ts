import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  answerRecipientKeyChallenge,
  generateRecipientKeyPair,
  recipientKeyFingerprint,
  recipientPublicKey,
} from "../src/index.js";

function readJson<T>(name: string): T {
  return JSON.parse(
    readFileSync(fileURLToPath(new URL(`../../fixtures/${name}`, import.meta.url)), "utf8"),
  ) as T;
}
const keypair = readJson<{ privateKeyPkcs8B64: string; publicKeyRawB64: string }>("keypair.json");
const challenge = readJson<{ fingerprint: string; nonce: string; envelope: string }>(
  "recipient-key-challenge.json",
);

describe("recipient key helpers", () => {
  it("generates a P-256 pair: a 65-byte raw public point and a PKCS#8 private half", async () => {
    const pair = await generateRecipientKeyPair();
    const pub = Buffer.from(pair.publicKeyRawBase64, "base64");
    expect(pub.length).toBe(65);
    expect(pub[0]).toBe(0x04);
    expect(Buffer.from(pair.privateKeyPkcs8Base64, "base64")[0]).toBe(0x30);
    expect(await recipientPublicKey(pair.privateKeyPkcs8Base64)).toBe(pair.publicKeyRawBase64);
    const again = await generateRecipientKeyPair();
    expect(again.publicKeyRawBase64).not.toBe(pair.publicKeyRawBase64);
  });

  it("fingerprints the committed fixture key the way the partner page shows it", async () => {
    // Written out by hand for the committed keypair.json: SHA-256 of the raw point, first 16 hex.
    expect(recipientKeyFingerprint(keypair.publicKeyRawB64)).toBe("91fa2aea473dbab6");
    expect(recipientKeyFingerprint(keypair.publicKeyRawB64)).toBe(challenge.fingerprint);
    expect(recipientKeyFingerprint(await recipientPublicKey(keypair.privateKeyPkcs8B64))).toBe(
      "91fa2aea473dbab6",
    );
  });

  it("answers the possession challenge with the nonce, and only with the right key", async () => {
    expect(await answerRecipientKeyChallenge(keypair.privateKeyPkcs8B64, challenge.envelope)).toBe(
      challenge.nonce,
    );
    const stranger = await generateRecipientKeyPair();
    await expect(
      answerRecipientKeyChallenge(stranger.privateKeyPkcs8Base64, challenge.envelope),
    ).rejects.toThrow();
  });

  it("refuses an envelope that carries no nonce", async () => {
    const { envelopes } = { envelopes: readJson<{ uid: string }>("envelopes.json") };
    await expect(
      answerRecipientKeyChallenge(keypair.privateKeyPkcs8B64, envelopes.uid),
    ).rejects.toThrow(/nonce/);
  });
});
