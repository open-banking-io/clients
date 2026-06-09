import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { fileURLToPath } from "node:url";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { OpenBankingClient } from "../src/index.js";

const fixtures = fileURLToPath(new URL("../../fixtures/", import.meta.url));
const readJson = (name: string) => JSON.parse(readFileSync(fixtures + name, "utf8"));

const API_KEY = readJson("credentials.json").apiKey as string;
const PRIVATE_KEY = readJson("keypair.json").privateKeyPkcs8B64 as string;

let server: Server;
let baseUrl: string;
let lastSyncBody: unknown = null;

function send(res: import("node:http").ServerResponse, status: number, body: unknown): void {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

beforeAll(async () => {
  server = createServer((req, res) => {
    // Enforce the API key — 401 otherwise.
    if (req.headers["x-api-key"] !== API_KEY) {
      send(res, 401, { error: "unauthorized" });
      return;
    }

    const url = new URL(req.url ?? "/", "http://localhost");
    const path = url.pathname;

    if (req.method === "GET" && path === "/api/accounts") {
      send(res, 200, readJson("api/accounts.json"));
      return;
    }
    if (req.method === "GET" && /^\/api\/accounts\/[^/]+\/transactions$/.test(path)) {
      send(res, 200, readJson("api/transactions.json"));
      return;
    }
    if (req.method === "GET" && path === "/api/connections") {
      send(res, 200, readJson("api/connections.json"));
      return;
    }
    if (req.method === "POST" && /^\/api\/accounts\/[^/]+\/sync$/.test(path)) {
      let raw = "";
      req.on("data", (c) => (raw += c));
      req.on("end", () => {
        lastSyncBody = JSON.parse(raw || "{}");
        send(res, 200, readJson("api/sync.json"));
      });
      return;
    }
    if (req.method === "POST" && path === "/api/sync") {
      let raw = "";
      req.on("data", (c) => (raw += c));
      req.on("end", () => {
        lastSyncBody = JSON.parse(raw || "{}");
        send(res, 200, readJson("api/sync-all.json"));
      });
      return;
    }

    send(res, 404, { error: "not found" });
  });

  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as AddressInfo).port;
  baseUrl = `http://127.0.0.1:${port}`;
});

afterAll(async () => {
  await new Promise<void>((resolve) => server.close(() => resolve()));
});

function client(apiKey = API_KEY): OpenBankingClient {
  return new OpenBankingClient({ apiBaseUrl: baseUrl, apiKey, privateKeyPkcs8: PRIVATE_KEY });
}

describe("integration against a mock API", () => {
  it("getAccounts decrypts iban/ownerName/displayName and 3 balances", async () => {
    const accounts = await client().getAccounts();
    expect(accounts).toHaveLength(1);
    const a = accounts[0]!;

    expect(a.iban).toBe("DK6466952001724927");
    expect(a.ownerName).toBe("Tatic ApS");
    expect(a.displayName).toBe("Drift");
    expect(a.balances).toHaveLength(3);

    const itbd = a.balances.find((b) => b.type === "ITBD");
    const itav = a.balances.find((b) => b.type === "ITAV");
    expect(itbd?.amount).toBe("828.13");
    expect(itav?.amount).toBe("633.90");
  });

  it("getTransactions decrypts amount and creditor", async () => {
    const page = await client().getTransactions("11111111-1111-4111-8111-111111111111", {
      limit: 50,
    });
    expect(page.total).toBe(1);
    const t = page.items[0]!;
    expect(t.amount).toBe("194.23");
    expect(t.creditorName).toBe("One.com");
  });

  it("getConnections works", async () => {
    const conns = await client().getConnections();
    expect(conns).toHaveLength(1);
    expect(conns[0]!.aspspName).toBe("Lunar");
    expect(conns[0]!.status).toBe("Active");
  });

  it("sync POSTs a body containing the decrypted uid", async () => {
    lastSyncBody = null;
    const result = await client().sync("11111111-1111-4111-8111-111111111111");
    expect(result.totalFetched).toBe(1);
    expect(lastSyncBody).toEqual({ uid: "c5d93aa7-5e23-4da0-ba88-42b9a584492c" });
  });

  it("syncAll POSTs items with the decrypted uid", async () => {
    lastSyncBody = null;
    const result = await client().syncAll();
    expect(result.accounts).toBe(1);
    expect(lastSyncBody).toEqual({
      items: [
        {
          accountId: "11111111-1111-4111-8111-111111111111",
          uid: "c5d93aa7-5e23-4da0-ba88-42b9a584492c",
        },
      ],
    });
  });

  it("a wrong API key throws", async () => {
    await expect(client("wrong-key").getAccounts()).rejects.toThrow();
  });

  it("a wrong private key throws on decryption", async () => {
    const bad = new OpenBankingClient({
      apiBaseUrl: baseUrl,
      apiKey: API_KEY,
      // A different valid P-256 PKCS#8 key — derives the wrong shared secret.
      privateKeyPkcs8:
        "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgdB9vjCwdrF1FckSNDHrw8M7PNNkpUB/tAK/EgAbZWvihRANCAASgg8XOKlU9VeYCee9+tQKtDSkFze10CRTA4b2gGKDlHIFw+QTf1AkjAjLfWqCJ4BctUqQtAYs0v0Y90Bw1wsBF",
    });
    await expect(bad.getAccounts()).rejects.toThrow();
  });
});
