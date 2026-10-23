import { describe, expect, it } from "vitest";

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { OpenBankingClient } from "../src/index.js";

const fixtures = fileURLToPath(new URL("../../fixtures/", import.meta.url));
const PRIVATE_KEY = (
  JSON.parse(readFileSync(fixtures + "keypair.json", "utf8")) as { privateKeyPkcs8B64: string }
).privateKeyPkcs8B64;

const opts = (apiBaseUrl: unknown) =>
  ({ apiBaseUrl, apiKey: "k", privateKeyPkcs8: PRIVATE_KEY }) as never;

describe("apiBaseUrl normalization", () => {
  it("trims surrounding whitespace", async () => {
    let seen = "";
    const client = new OpenBankingClient({
      apiBaseUrl: "  https://example.test/  ",
      apiKey: "k",
      privateKeyPkcs8: PRIVATE_KEY,
      fetch: ((url: string) => {
        seen = url;
        return Promise.resolve(
          new Response("[]", { headers: { "content-type": "application/json" } }),
        );
      }) as never,
    });
    await client.getAccounts();
    expect(seen).toBe("https://example.test/api/accounts");
  });

  it.each(["open-banking.io", "//open-banking.io", "ftp://open-banking.io"])(
    "rejects %s (no http scheme)",
    (bad) => {
      expect(() => new OpenBankingClient(opts(bad))).toThrow(/http/i);
    },
  );

  it.each(["http://open-banking.io", "http://192.168.1.10:8080", "http://localhost.evil.test"])(
    "rejects cleartext %s",
    (bad) => {
      expect(() => new OpenBankingClient(opts(bad))).toThrow(/https/i);
    },
  );

  it.each(["http://localhost:8080", "http://127.0.0.1:8080", "http://[::1]:8080"])(
    "allows cleartext on loopback %s",
    (ok) => {
      expect(() => new OpenBankingClient(opts(ok))).not.toThrow();
    },
  );

  it.each([123, null, undefined, {}, []])("rejects non-string %s", (bad) => {
    expect(() => new OpenBankingClient(opts(bad))).toThrow(/apiBaseUrl/);
  });
});
