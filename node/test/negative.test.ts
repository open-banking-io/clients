import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { fileURLToPath } from "node:url";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { decryptEnvelope, importPrivateKey, OpenBankingClient } from "../src/index.js";

const fixtures = fileURLToPath(new URL("../../fixtures/", import.meta.url));
const readJson = <T = unknown>(name: string): T =>
  JSON.parse(readFileSync(fixtures + name, "utf8")) as T;

const PRIVATE_KEY = readJson<{ privateKeyPkcs8B64: string }>("keypair.json").privateKeyPkcs8B64;
const ENVELOPES = readJson<{ account: string }>("envelopes.json");

function bytesToBase64(bytes: Uint8Array): string {
  return Buffer.from(bytes).toString("base64");
}

describe("envelope decryption — malformed input", () => {
  it("rejects a too-short envelope (below the header minimum)", async () => {
    const priv = await importPrivateKey(PRIVATE_KEY);
    // Anything shorter than version(1)+point(65)+nonce(12)+tag(16) = 94 bytes is invalid.
    const tooShort = bytesToBase64(new Uint8Array(10).fill(0x01));
    await expect(decryptEnvelope(priv, tooShort)).rejects.toThrow(/invalid or unsupported/i);
  });

  it("rejects an empty envelope", async () => {
    const priv = await importPrivateKey(PRIVATE_KEY);
    await expect(decryptEnvelope(priv, "")).rejects.toThrow(/invalid or unsupported/i);
  });

  it("rejects a wrong version byte even when the length is sufficient", async () => {
    const priv = await importPrivateKey(PRIVATE_KEY);
    const bytes = new Uint8Array(1 + 65 + 12 + 16 + 4);
    bytes[0] = 0x02; // unsupported version
    await expect(decryptEnvelope(priv, bytesToBase64(bytes))).rejects.toThrow(
      /invalid or unsupported/i,
    );
  });

  it("rejects a truncated body of a real envelope (GCM tag mismatch / bad point)", async () => {
    const priv = await importPrivateKey(PRIVATE_KEY);
    // Take a valid envelope and lop off its trailing ciphertext bytes: still long enough to pass
    // the header check, but the AES-GCM authentication tag will no longer verify.
    const full = Buffer.from(ENVELOPES.account, "base64");
    const truncated = bytesToBase64(new Uint8Array(full.subarray(0, full.length - 8)));
    await expect(decryptEnvelope(priv, truncated)).rejects.toThrow();
  });
});

describe("private key import — malformed input", () => {
  it("rejects a non-base64 / garbage PKCS#8 key", async () => {
    await expect(importPrivateKey("not-a-real-key")).rejects.toThrow();
  });

  it("rejects a well-formed base64 blob that is not a valid PKCS#8 key", async () => {
    const garbage = Buffer.from("this is not pkcs8 at all, just some bytes").toString("base64");
    await expect(importPrivateKey(garbage)).rejects.toThrow();
  });
});

describe("client — HTTP non-2xx propagation", () => {
  let server: Server;
  let baseUrl: string;

  beforeAll(async () => {
    server = createServer((_req, res) => {
      res.writeHead(500, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "boom" }));
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const port = (server.address() as AddressInfo).port;
    baseUrl = `http://127.0.0.1:${port}`;
  });

  afterAll(async () => {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });

  it("throws with status text when the API returns 500", async () => {
    const client = new OpenBankingClient({
      apiBaseUrl: baseUrl,
      apiKey: "any-key",
      privateKeyPkcs8: PRIVATE_KEY,
    });
    await expect(client.getAccounts()).rejects.toThrow(/500/);
  });
});
