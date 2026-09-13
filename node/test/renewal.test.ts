import { webcrypto } from "node:crypto";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  buildAuthorizeUrl,
  closeReplacedConsents,
  OpenBankingClient,
  SyncError,
} from "../src/index.js";

const fixtures = fileURLToPath(new URL("../../fixtures/", import.meta.url));
const readJson = <T = unknown>(name: string): T =>
  JSON.parse(readFileSync(fixtures + name, "utf8")) as T;

const PRIVATE_KEY = readJson<{ privateKeyPkcs8B64: string }>("keypair.json").privateKeyPkcs8B64;
const ACCOUNT = "11111111-1111-4111-8111-111111111111";
const UID = "c5d93aa7-5e23-4da0-ba88-42b9a584492c";
const PUBLIC_KEY = readJson<{ publicKeyRawB64: string }>("keypair.json").publicKeyRawB64;

async function seal(payload: unknown): Promise<string> {
  const { subtle } = webcrypto;
  const recipient = await subtle.importKey(
    "raw",
    Buffer.from(PUBLIC_KEY, "base64"),
    { name: "ECDH", namedCurve: "P-256" },
    false,
    [],
  );
  const eph = await subtle.generateKey({ name: "ECDH", namedCurve: "P-256" }, true, ["deriveBits"]);
  const shared = await subtle.deriveBits({ name: "ECDH", public: recipient }, eph.privateKey, 256);
  const hkdf = await subtle.importKey("raw", shared, "HKDF", false, ["deriveKey"]);
  const aes = await subtle.deriveKey(
    {
      name: "HKDF",
      hash: "SHA-256",
      salt: new Uint8Array(32),
      info: new TextEncoder().encode("bank.core.ci/zk/v1"),
    },
    hkdf,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt"],
  );
  const nonce = webcrypto.getRandomValues(new Uint8Array(12));
  const sealed = new Uint8Array(
    await subtle.encrypt(
      { name: "AES-GCM", iv: nonce, tagLength: 128 },
      aes,
      new TextEncoder().encode(JSON.stringify(payload)),
    ),
  );
  const point = new Uint8Array(await subtle.exportKey("raw", eph.publicKey));
  const ct = sealed.subarray(0, sealed.length - 16);
  const tag = sealed.subarray(sealed.length - 16);
  return Buffer.concat([Buffer.from([1]), point, nonce, tag, ct]).toString("base64");
}

interface Call {
  method: string;
  path: string;
  headers: Record<string, string>;
  body: string;
}

type Route = (call: Call, index: number) => Response | undefined;

function json(status: number, body: unknown, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

const SIBLING = "66666666-6666-4666-8666-666666666666";

function twoAccounts(): unknown[] {
  const [account] = readJson<Record<string, unknown>[]>("api/accounts.json");
  return [account, { ...account, id: SIBLING }];
}

const RENEWED_UID = "0e8a4c7b-9d1f-4c2a-8b6e-renewed00001";

function readsThenRenews(renewedEnc: string): () => unknown[] {
  let reads = 0;
  return () => {
    const [account, sibling] = twoAccounts() as Record<string, unknown>[];
    reads += 1;
    return reads === 1 ? [account, sibling] : [{ ...account, uidEnc: renewedEnc }, sibling];
  };
}

function lapsedAccount(id: string): Record<string, unknown> {
  const [account] = readJson<Record<string, unknown>[]>("api/accounts.json");
  return { ...account, id, needsReconnect: true, uidEnc: null };
}

const userinfoFor = (clientId: string) =>
  json(200, {
    sub: "wl:7:connect:ada@example.com",
    email: "ada@example.com",
    partner_id: "7",
    client_id: clientId,
    scope: "accounts.read",
    expires_at: null,
  });

function stub(route: Route, accounts: () => unknown = () => readJson("api/accounts.json")) {
  const calls: Call[] = [];
  const fetchImpl: typeof globalThis.fetch = (input, init) => {
    const url = new URL(input instanceof Request ? input.url : String(input));
    const body = init?.body;
    const call: Call = {
      method: init?.method ?? "GET",
      path: url.pathname,
      headers: Object.fromEntries(new Headers(init?.headers).entries()),
      body:
        typeof body === "string" ? body : body instanceof URLSearchParams ? body.toString() : "",
    };
    calls.push(call);
    if (call.method === "GET" && call.path === "/api/accounts")
      return Promise.resolve(json(200, accounts()));
    const answer = route(call, calls.filter((c) => c.path === call.path).length - 1);
    return Promise.resolve(answer ?? json(404, {}));
  };
  const client = new OpenBankingClient({
    apiBaseUrl: "http://api.test",
    apiKey: "ebk_test",
    privateKeyPkcs8: PRIVATE_KEY,
    fetch: fetchImpl,
  });
  return { client, calls, fetchImpl };
}

describe("connections", () => {
  it("carries isLive and accountIds from the service", async () => {
    const { client } = stub((c) =>
      c.path === "/api/connections" ? json(200, readJson("api/connections.json")) : undefined,
    );

    const [connection] = await client.getConnections();

    expect(connection!.isLive).toBe(false);
    expect(connection!.accountIds).toEqual([ACCOUNT]);
  });

  it("trusts the service's isLive over the local clock", async () => {
    const { client } = stub((c) =>
      c.path === "/api/connections"
        ? json(200, [
            {
              sessionId: "a",
              aspspName: "Lunar",
              aspspCountry: "DK",
              validUntil: "2020-01-01T00:00:00Z",
              status: "Active",
              accountCount: 1,
              isLive: true,
            },
          ])
        : undefined,
    );

    const [connection] = await client.getConnections();

    expect(connection!.isLive).toBe(true);
  });

  it("decides isLive from status and validUntil when an older service omits it", async () => {
    const future = new Date(Date.now() + 86_400_000).toISOString();
    const { client } = stub((c) =>
      c.path === "/api/connections"
        ? json(200, [
            {
              sessionId: "a",
              aspspName: "Lunar",
              aspspCountry: "DK",
              validUntil: future,
              status: "Active",
              accountCount: 1,
            },
            {
              sessionId: "b",
              aspspName: "Lunar",
              aspspCountry: "DK",
              validUntil: "2020-01-01T00:00:00Z",
              status: "Active",
              accountCount: 1,
            },
            {
              sessionId: "c",
              aspspName: "Lunar",
              aspspCountry: "DK",
              validUntil: future,
              status: "Revoked",
              accountCount: 1,
            },
          ])
        : undefined,
    );

    const live = (await client.getConnections()).map((c) => [c.sessionId, c.isLive, c.accountIds]);

    expect(live).toEqual([
      ["a", true, []],
      ["b", false, []],
      ["c", false, []],
    ]);
  });
});

describe("sync failures", () => {
  it("syncAll returns every failure with its reason and bank code", async () => {
    const { client } = stub((c) =>
      c.path === "/api/sync" ? json(200, readJson("api/sync-all-failures.json")) : undefined,
    );

    const result = await client.syncAll();

    expect(result.accounts).toBe(1);
    expect(result.newTransactions).toBe(4);
    expect(result.failures).toEqual([
      {
        accountId: "33333333-3333-4333-8333-333333333333",
        reason: "reconnect_needed",
        bankErrorCode: "ASPSP_ACCOUNT_NOT_ACCESSIBLE",
      },
      {
        accountId: "44444444-4444-4444-8444-444444444444",
        reason: "psu_present_required",
        bankErrorCode: "PSU_HEADER_NOT_PROVIDED",
      },
      {
        accountId: "55555555-5555-4555-8555-555555555555",
        reason: "rate_limited",
        bankErrorCode: null,
      },
    ]);
  });

  it("a refused single sync throws SyncError with the problem's reason, code and Retry-After", async () => {
    const { client } = stub((c) =>
      c.method === "POST"
        ? json(
            429,
            { status: 429, reason: "rate_limited", bankErrorCode: "ASPSP_RATE_LIMIT_EXCEEDED" },
            { "Retry-After": "3600" },
          )
        : undefined,
    );

    const error = await client.sync(ACCOUNT).catch((e: unknown) => e);

    expect(error).toBeInstanceOf(SyncError);
    expect(error).toMatchObject({
      status: 429,
      reason: "rate_limited",
      bankErrorCode: "ASPSP_RATE_LIMIT_EXCEEDED",
      retryAfterSeconds: 3600,
    });
  });

  it("a body that is not a problem still throws SyncError, with no reason", async () => {
    const { client } = stub((c) =>
      c.method === "POST" ? new Response("gateway", { status: 502 }) : undefined,
    );

    const error = await client.sync(ACCOUNT).catch((e: unknown) => e);

    expect(error).toMatchObject({
      name: "SyncError",
      status: 502,
      reason: null,
      bankErrorCode: null,
    });
  });
});

describe("uid_outdated", () => {
  const outdated = () => json(409, { status: 409, reason: "uid_outdated" });

  it("sync re-reads the account and retries exactly once", async () => {
    const { client, calls } = stub((c, i) =>
      c.method === "POST"
        ? i === 0
          ? outdated()
          : json(200, { newTransactions: 2, totalFetched: 5 })
        : undefined,
    );

    const result = await client.sync(ACCOUNT);

    expect(result).toEqual({ newTransactions: 2, totalFetched: 5 });
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "GET /api/accounts",
      `POST /api/accounts/${ACCOUNT}/sync`,
      "GET /api/accounts",
      `POST /api/accounts/${ACCOUNT}/sync`,
    ]);
  });

  it("sync gives up after the second uid_outdated", async () => {
    const { client, calls } = stub((c) => (c.method === "POST" ? outdated() : undefined));

    await expect(client.sync(ACCOUNT)).rejects.toMatchObject({ reason: "uid_outdated" });
    expect(calls.filter((c) => c.method === "POST")).toHaveLength(2);
  });

  it("syncAll retries only the outdated accounts once, with the re-read uid, and merges the counts", async () => {
    const renewedEnc = await seal({ uid: RENEWED_UID });
    const { client, calls } = stub((c, i) => {
      if (c.path !== "/api/sync") return undefined;
      if (i === 0)
        return json(200, {
          accounts: 1,
          newTransactions: 3,
          failures: [
            { accountId: ACCOUNT, reason: "uid_outdated" },
            { accountId: "33333333-3333-4333-8333-333333333333", reason: "reconnect_needed" },
          ],
        });
      return json(200, { accounts: 1, newTransactions: 7, failures: [] });
    }, readsThenRenews(renewedEnc));

    const result = await client.syncAll();

    const posts = calls.filter((c) => c.path === "/api/sync");
    expect(posts).toHaveLength(2);
    expect(JSON.parse(posts[0]!.body)).toEqual({
      items: [
        { accountId: ACCOUNT, uid: UID },
        { accountId: SIBLING, uid: UID },
      ],
    });
    expect(JSON.parse(posts[1]!.body)).toEqual({
      items: [{ accountId: ACCOUNT, uid: RENEWED_UID }],
    });
    expect(result).toEqual({
      accounts: 2,
      newTransactions: 10,
      failures: [
        {
          accountId: "33333333-3333-4333-8333-333333333333",
          reason: "reconnect_needed",
          bankErrorCode: null,
        },
      ],
    });
  });

  it("syncAll reports uid_outdated when the retry is refused again", async () => {
    const { client, calls } = stub((c) =>
      c.path === "/api/sync"
        ? json(200, {
            accounts: 0,
            newTransactions: 0,
            failures: [{ accountId: ACCOUNT, reason: "uid_outdated" }],
          })
        : undefined,
    );

    const result = await client.syncAll();

    expect(calls.filter((c) => c.path === "/api/sync")).toHaveLength(2);
    expect(result.failures).toEqual([
      { accountId: ACCOUNT, reason: "uid_outdated", bankErrorCode: null },
    ]);
  });
});

describe("lapsed consents", () => {
  it("syncAll reports an account whose consent needs renewing instead of dropping it", async () => {
    const LAPSED = "77777777-7777-4777-8777-777777777777";
    const { client, calls } = stub(
      (c) =>
        c.path === "/api/sync"
          ? json(200, { accounts: 1, newTransactions: 0, failures: [] })
          : undefined,
      () => [...readJson<unknown[]>("api/accounts.json"), lapsedAccount(LAPSED)],
    );

    const result = await client.syncAll();

    const post = calls.find((c) => c.path === "/api/sync")!;
    expect(JSON.parse(post.body)).toEqual({ items: [{ accountId: ACCOUNT, uid: UID }] });
    expect(result.failures).toEqual([
      { accountId: LAPSED, reason: "reconnect_needed", bankErrorCode: null },
    ]);
  });

  it("an account whose consent lapsed between the two reads is reported reconnect_needed, not uid_outdated", async () => {
    let reads = 0;
    const { client, calls } = stub(
      (c) =>
        c.path === "/api/sync"
          ? json(200, {
              accounts: 0,
              newTransactions: 0,
              failures: [{ accountId: ACCOUNT, reason: "uid_outdated" }],
            })
          : undefined,
      () => (++reads === 1 ? readJson("api/accounts.json") : [lapsedAccount(ACCOUNT)]),
    );

    const result = await client.syncAll();

    expect(calls.filter((c) => c.path === "/api/sync")).toHaveLength(1);
    expect(result.failures).toEqual([
      { accountId: ACCOUNT, reason: "reconnect_needed", bankErrorCode: null },
    ]);
  });
});

describe("forwarded PSU headers", () => {
  const psu = {
    ipAddress: "203.0.113.7",
    userAgent: "Mozilla/5.0",
    acceptLanguage: "da-DK",
  };

  it("sync and syncAll send the present user's headers", async () => {
    const { client, calls } = stub((c) =>
      c.method === "POST"
        ? c.path === "/api/sync"
          ? json(200, { accounts: 1, newTransactions: 0, failures: [] })
          : json(200, { newTransactions: 0, totalFetched: 0 })
        : undefined,
    );

    await client.sync(ACCOUNT, { psu });
    await client.syncAll({ psu });

    for (const post of calls.filter((c) => c.method === "POST")) {
      expect(post.headers["x-psu-ip-address"]).toBe("203.0.113.7");
      expect(post.headers["x-psu-user-agent"]).toBe("Mozilla/5.0");
      expect(post.headers["x-psu-accept-language"]).toBe("da-DK");
      expect(post.headers["x-psu-referer"]).toBeUndefined();
      expect(post.headers["x-api-key"]).toBe("ebk_test");
    }
  });

  it("sends none when the address or user agent is missing", async () => {
    const { client, calls } = stub((c) =>
      c.method === "POST"
        ? json(200, { accounts: 1, newTransactions: 0, failures: [] })
        : undefined,
    );

    await client.syncAll({ psu: { ipAddress: "203.0.113.7", userAgent: "" } });
    await client.syncAll({
      psu: { ipAddress: undefined as unknown as string, userAgent: "Mozilla/5.0" },
    });

    for (const post of calls.filter((c) => c.method === "POST")) {
      expect(Object.keys(post.headers).filter((h) => h.startsWith("x-psu-"))).toEqual([]);
    }
  });

  it("sends none without the option", async () => {
    const { client, calls } = stub((c) =>
      c.method === "POST"
        ? json(200, { accounts: 1, newTransactions: 0, failures: [] })
        : undefined,
    );

    await client.syncAll();

    const post = calls.find((c) => c.method === "POST")!;
    expect(Object.keys(post.headers).filter((h) => h.startsWith("x-psu-"))).toEqual([]);
  });
});

describe("renewal", () => {
  it("buildAuthorizeUrl carries renew_connection", () => {
    const url = new URL(
      buildAuthorizeUrl({
        issuer: "https://open-banking.io",
        clientId: "obc_x",
        redirectUri: "https://partner.test/cb",
        state: "s",
        codeChallenge: "c",
        renewConnection: "22222222-2222-4222-8222-222222222222",
      }),
    );

    expect(url.searchParams.get("renew_connection")).toBe("22222222-2222-4222-8222-222222222222");
  });

  it("closeReplacedConsents reads open-consents only, and closes them in the revoke of the previous key", async () => {
    const SESSION_ENC = await seal({ sessionId: "eb-old-1" });
    const { calls, fetchImpl } = stub((c) => {
      if (c.path === "/api/connections/open-consents")
        return json(200, [
          {
            connectionId: "old-1",
            aspspName: "Lunar",
            aspspCountry: "DK",
            endsAt: "2026-10-01T00:00:00Z",
            sessionIdEnc: SESSION_ENC,
            superseded: true,
            weCanEndIt: true,
          },
          {
            connectionId: "old-1",
            aspspName: "Lunar",
            aspspCountry: "DK",
            endsAt: "2026-10-01T00:00:00Z",
            sessionIdEnc: SESSION_ENC,
            superseded: true,
            weCanEndIt: true,
          },
          {
            connectionId: "old-2",
            aspspName: "Lunar",
            aspspCountry: "DK",
            endsAt: "2026-10-01T00:00:00Z",
            sessionIdEnc: null,
            superseded: true,
            weCanEndIt: false,
          },
        ]);
      if (c.path === "/oauth/revoke") return new Response(null, { status: 200 });
      if (c.path === "/oauth/userinfo") return userinfoFor("obc_x");
      return undefined;
    });

    const result = await closeReplacedConsents({
      issuer: "http://api.test",
      clientId: "obc_x",
      clientSecret: "secret",
      token: "ebk_previous",
      privateKey: PRIVATE_KEY,
      fetch: fetchImpl,
    });

    expect(result).toEqual({ closed: 1, failed: 1, revoked: true });
    expect(calls.map((c) => c.path)).toEqual([
      "/oauth/userinfo",
      "/api/connections/open-consents",
      "/oauth/revoke",
    ]);
    expect(calls[1]!.headers["x-api-key"]).toBe("ebk_previous");
    const form = new URLSearchParams(calls[2]!.body);
    expect(form.get("token")).toBe("ebk_previous");
    expect(form.getAll("connection_id")).toEqual(["old-1"]);
    expect(form.getAll("eb_session_id")).toEqual(["eb-old-1"]);
  });

  it("closeReplacedConsents neither closes nor revokes when it cannot read the list", async () => {
    const { calls, fetchImpl } = stub((c) => {
      if (c.path === "/oauth/userinfo") return userinfoFor("obc_x");
      return c.path === "/api/connections/open-consents" ? json(401, {}) : undefined;
    });

    const result = await closeReplacedConsents({
      issuer: "http://api.test",
      clientId: "obc_x",
      clientSecret: "secret",
      token: "ebk_previous",
      privateKey: PRIVATE_KEY,
      fetch: fetchImpl,
    });

    expect(result).toEqual({ closed: 0, failed: 0, revoked: false });
    expect(calls.map((c) => c.path)).toEqual(["/oauth/userinfo", "/api/connections/open-consents"]);
  });

  it("closeReplacedConsents reports the key alive when the revoke is refused", async () => {
    const SESSION_ENC = await seal({ sessionId: "eb-old-1" });
    const { fetchImpl } = stub((c) => {
      if (c.path === "/api/connections/open-consents")
        return json(200, [
          {
            connectionId: "old-1",
            aspspName: "Lunar",
            aspspCountry: "DK",
            endsAt: "2026-10-01T00:00:00Z",
            sessionIdEnc: SESSION_ENC,
            superseded: true,
            weCanEndIt: true,
          },
        ]);
      if (c.path === "/oauth/revoke")
        return json(400, { error: "invalid_request", error_description: "unknown connection" });
      if (c.path === "/oauth/userinfo") return userinfoFor("obc_x");
      return undefined;
    });

    const result = await closeReplacedConsents({
      issuer: "http://api.test",
      clientId: "obc_x",
      clientSecret: "secret",
      token: "ebk_previous",
      privateKey: PRIVATE_KEY,
      fetch: fetchImpl,
    });

    expect(result).toEqual({ closed: 0, failed: 1, revoked: false });
  });

  it("closeReplacedConsents closes nothing with a key issued to another client, or one already gone", async () => {
    for (const answer of [userinfoFor("obc_other"), json(401, { error: "invalid_token" })]) {
      const { calls, fetchImpl } = stub((c) =>
        c.path === "/oauth/userinfo" ? answer.clone() : undefined,
      );

      const result = await closeReplacedConsents({
        issuer: "http://api.test",
        clientId: "obc_x",
        clientSecret: "secret",
        token: "ebk_previous",
        privateKey: PRIVATE_KEY,
        fetch: fetchImpl,
      });

      expect(result).toEqual({ closed: 0, failed: 0, revoked: false });
      expect(calls.map((c) => c.path)).toEqual(["/oauth/userinfo"]);
    }
  });

  it("closeReplacedConsents throws on an outage instead of giving up the only key that can close", async () => {
    for (const failing of ["/oauth/userinfo", "/api/connections/open-consents", "/oauth/revoke"]) {
      const { calls, fetchImpl } = stub((c) => {
        if (c.path === failing) return json(503, { error: "temporarily_unavailable" });
        if (c.path === "/oauth/userinfo") return userinfoFor("obc_x");
        if (c.path === "/api/connections/open-consents") return json(200, []);
        if (c.path === "/oauth/revoke") return new Response(null, { status: 200 });
        return undefined;
      });

      await expect(
        closeReplacedConsents({
          issuer: "http://api.test",
          clientId: "obc_x",
          clientSecret: "secret",
          token: "ebk_previous",
          privateKey: PRIVATE_KEY,
          fetch: fetchImpl,
        }),
      ).rejects.toThrow();
      expect(calls.at(-1)!.path).toBe(failing);
    }
  });
});

describe("accounts that need a reconnect", () => {
  const V2_ENVELOPE = Buffer.concat([Buffer.from([2]), Buffer.alloc(120, 7)]).toString("base64");

  const needingReconnect = (uidEnc: string | null) => () => [
    {
      ...readJson<Record<string, unknown>[]>("api/accounts.json")[0],
      needsReconnect: true,
      uidEnc,
    },
  ];

  it("a single sync throws a typed reconnect_needed without calling the service", async () => {
    for (const uidEnc of [null, V2_ENVELOPE]) {
      const { client, calls } = stub(() => undefined, needingReconnect(uidEnc));

      const error = await client.sync(ACCOUNT).catch((e: unknown) => e);

      expect(error).toBeInstanceOf(SyncError);
      expect(error).toMatchObject({ reason: "reconnect_needed" });
      expect(calls.filter((c) => c.method === "POST")).toEqual([]);
    }
  });

  it("syncAll reports an envelope it cannot open as reconnect_needed, and posts nothing when nothing is left", async () => {
    const { client, calls } = stub(() => undefined, needingReconnect(V2_ENVELOPE));

    const result = await client.syncAll();

    expect(result).toEqual({
      accounts: 0,
      newTransactions: 0,
      failures: [{ accountId: ACCOUNT, reason: "reconnect_needed", bankErrorCode: null }],
    });
    expect(calls.filter((c) => c.method === "POST")).toEqual([]);
  });
});
