import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it, vi } from "vitest";
import {
  buildAuthorizeUrl,
  bundleFromToken,
  CONNECT_RELAY_FIELDS,
  createPkce,
  createState,
  discover,
  exchangeCode,
  OAuthError,
  OpenBankingClient,
  parseRelay,
  pkceChallenge,
  RelayError,
  revokeToken,
  userinfo,
} from "../src/index.js";

const ISSUER = "https://staging.open-banking.io";

function jsonResponse(
  status: number,
  body: unknown,
  headers: Record<string, string> = {},
): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

type Call = { url: string; init: RequestInit };
function fakeFetch(respond: (call: Call) => Response): {
  fetch: typeof fetch;
  calls: Call[];
  first: () => Call;
} {
  const calls: Call[] = [];
  const impl = vi.fn((input: string | URL | Request, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    const call = { url, init: init ?? {} };
    calls.push(call);
    return Promise.resolve(respond(call));
  });
  const first = (): Call => {
    const call = calls[0];
    if (!call) throw new Error("fetch was not called");
    return call;
  };
  return { fetch: impl, calls, first };
}

const fixtures = fileURLToPath(new URL("../../fixtures/", import.meta.url));
const readJson = <T = unknown>(name: string): T =>
  JSON.parse(readFileSync(fixtures + name, "utf8")) as T;
const PRIVATE_KEY = readJson<{ privateKeyPkcs8B64: string }>("keypair.json").privateKeyPkcs8B64;

describe("PKCE and state", () => {
  it("derives the RFC 7636 appendix B challenge", () => {
    expect(pkceChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")).toBe(
      "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
    );
  });

  it("creates a 43-char base64url verifier with a matching S256 challenge", () => {
    const pkce = createPkce();
    expect(pkce.verifier).toMatch(/^[A-Za-z0-9_-]{43}$/);
    expect(pkce.method).toBe("S256");
    expect(pkce.challenge).toBe(createHash("sha256").update(pkce.verifier).digest("base64url"));
    expect(createPkce().verifier).not.toBe(pkce.verifier);
  });

  it("creates unpredictable states", () => {
    expect(createState()).toMatch(/^[A-Za-z0-9_-]{22}$/);
    expect(createState()).not.toBe(createState());
  });
});

describe("buildAuthorizeUrl", () => {
  it("builds the exact authorize request", () => {
    const url = new URL(
      buildAuthorizeUrl({
        issuer: ISSUER + "/",
        clientId: "obc_demo",
        redirectUri: "https://app.example.com/callback",
        state: "s123",
        codeChallenge: "c456",
        loginHint: "alice@example.com",
        challenge: "pin_code",
        uiLocales: "da-DK en",
      }),
    );
    expect(url.origin + url.pathname).toBe(ISSUER + "/oauth/authorize");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      response_type: "code",
      client_id: "obc_demo",
      redirect_uri: "https://app.example.com/callback",
      scope: "accounts.read",
      state: "s123",
      code_challenge: "c456",
      code_challenge_method: "S256",
      response_mode: "form_post",
      login_hint: "alice@example.com",
      challenge: "pin_code",
      ui_locales: "da-DK en",
    });
  });

  it("omits the optional hints when not given", () => {
    const url = new URL(
      buildAuthorizeUrl({
        issuer: ISSUER,
        clientId: "c",
        redirectUri: "https://x/cb",
        state: "s",
        codeChallenge: "ch",
      }),
    );
    expect(url.searchParams.has("login_hint")).toBe(false);
    expect(url.searchParams.has("challenge")).toBe(false);
    expect(url.searchParams.has("ui_locales")).toBe(false);
  });
});

describe("parseRelay", () => {
  const relay = { code: "code1", state: "s123", iss: ISSUER, privateKey: "pk", publicKey: "pub" };

  it("names the wire fields in order", () => {
    expect([...CONNECT_RELAY_FIELDS]).toEqual(["code", "state", "iss", "privateKey", "publicKey"]);
  });

  it("accepts a form body, URLSearchParams, a record and JSON", () => {
    const expected = { ...relay };
    expect(
      parseRelay(new URLSearchParams(relay).toString(), { expectedState: "s123", issuer: ISSUER }),
    ).toEqual(expected);
    expect(
      parseRelay(new URLSearchParams(relay), { expectedState: "s123", issuer: ISSUER }),
    ).toEqual(expected);
    expect(parseRelay(relay, { expectedState: "s123", issuer: ISSUER })).toEqual(expected);
    expect(parseRelay(JSON.stringify(relay), { expectedState: "s123", issuer: ISSUER })).toEqual(
      expected,
    );
  });

  it("rejects a state mismatch, including a length mismatch and an empty expectation", () => {
    expect(() => parseRelay(relay, { expectedState: "s124" })).toThrow(RelayError);
    expect(() => parseRelay(relay, { expectedState: "s12" })).toThrow(/state/);
    expect(() => parseRelay(relay, { expectedState: "" })).toThrow(/state/);
    try {
      parseRelay(relay, { expectedState: "nope" });
    } catch (e) {
      expect((e as RelayError).code).toBe("state_mismatch");
    }
  });

  it("checks the issuer when asked, ignoring trailing slashes, and tolerates its absence otherwise", () => {
    expect(
      parseRelay({ ...relay, iss: ISSUER + "/" }, { expectedState: "s123", issuer: ISSUER }).iss,
    ).toBe(ISSUER + "/");
    expect(() =>
      parseRelay(
        { ...relay, iss: "https://evil.example" },
        { expectedState: "s123", issuer: ISSUER },
      ),
    ).toThrow(/issuer/);
    expect(() =>
      parseRelay({ ...relay, iss: "" }, { expectedState: "s123", issuer: ISSUER }),
    ).toThrow(/iss/);
    expect(() =>
      parseRelay({ ...relay, iss: "" }, { expectedState: "s123", requireIss: true }),
    ).toThrow(/iss/);
    expect(parseRelay({ ...relay, iss: "" }, { expectedState: "s123" }).iss).toBe("");
  });

  it("requires the code and the private key", () => {
    expect(() => parseRelay({ ...relay, code: "" }, { expectedState: "s123" })).toThrow(/code/);
    expect(() =>
      parseRelay({ ...relay, privateKey: undefined }, { expectedState: "s123" }),
    ).toThrow(/private key/);
  });

  it("names a cancel as access_denied, the normal outcome it is", () => {
    try {
      parseRelay(
        {
          error: "access_denied",
          error_description: "the user declined the authorization",
          state: "s123",
        },
        { expectedState: "s123" },
      );
      expect.unreachable();
    } catch (e) {
      expect((e as RelayError).code).toBe("access_denied");
      expect((e as RelayError).error).toBe("access_denied");
    }
  });

  it("surfaces an OAuth error relay before anything else", () => {
    try {
      parseRelay(
        { error: "invalid_scope", error_description: "unsupported scope", state: "s123" },
        { expectedState: "s123" },
      );
      expect.unreachable();
    } catch (e) {
      const err = e as RelayError;
      expect(err.code).toBe("oauth_error");
      expect(err.error).toBe("invalid_scope");
      expect(err.errorDescription).toBe("unsupported scope");
      expect(err.message).toBe("invalid_scope: unsupported scope");
    }
  });
});

describe("exchangeCode", () => {
  const token = {
    access_token: "ebk_key",
    token_type: "Bearer",
    expires_in: 31536000,
    scope: "accounts.read",
    apiKey: "ebk_key",
    apiBaseUrl: ISSUER,
    user: "wl:p:connect:alice@example.com",
  };

  it("posts the form-encoded request with HTTP Basic client authentication", async () => {
    const { fetch, calls, first } = fakeFetch(() => jsonResponse(200, token));

    const result = await exchangeCode({
      issuer: ISSUER,
      clientId: "obc_demo",
      clientSecret: "s3cret:with/odd chars",
      code: "code1",
      codeVerifier: "ver",
      redirectUri: "https://app.example.com/callback",
      fetch,
    });

    expect(calls).toHaveLength(1);
    const call = first();
    expect(call.url).toBe(ISSUER + "/oauth/token");
    expect(call.init.method).toBe("POST");
    const headers = call.init.headers as Record<string, string>;
    expect(headers["Content-Type"]).toBe("application/x-www-form-urlencoded");
    expect(headers.Authorization).toBe(
      "Basic " + Buffer.from("obc_demo:s3cret%3Awith%2Fodd%20chars").toString("base64"),
    );
    expect(headers["User-Agent"]).toMatch(/^open-banking-io\/node\//);
    expect(call.init.body).toBe(
      "grant_type=authorization_code&code=code1&code_verifier=ver&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback",
    );
    expect(call.init.signal).toBeInstanceOf(AbortSignal);
    expect(result).toEqual({
      accessToken: "ebk_key",
      tokenType: "Bearer",
      expiresIn: 31536000,
      scope: "accounts.read",
      apiKey: "ebk_key",
      apiBaseUrl: ISSUER,
      user: "wl:p:connect:alice@example.com",
    });
  });

  it("maps an RFC 6749 error body to OAuthError", async () => {
    const { fetch } = fakeFetch(() =>
      jsonResponse(400, {
        error: "invalid_grant",
        error_description: "invalid or expired authorization code",
      }),
    );

    const promise = exchangeCode({
      issuer: ISSUER,
      clientId: "c",
      clientSecret: "s",
      code: "x",
      codeVerifier: "v",
      fetch,
    });
    await expect(promise).rejects.toBeInstanceOf(OAuthError);
    await expect(promise).rejects.toMatchObject({ status: 400, error: "invalid_grant" });
  });

  it("reads a first-generation response that only carries apiKey", async () => {
    const { fetch } = fakeFetch(() =>
      jsonResponse(200, { apiKey: "ebk_old", apiBaseUrl: ISSUER, user: "u" }),
    );

    const result = await exchangeCode({
      issuer: ISSUER,
      clientId: "c",
      clientSecret: "s",
      code: "x",
      codeVerifier: "v",
      fetch,
    });

    expect(result.accessToken).toBe("ebk_old");
    expect(result.tokenType).toBe("Bearer");
  });

  it("aborts after the timeout", async () => {
    const hanging = vi.fn(
      (_url: string, init?: RequestInit) =>
        new Promise<Response>((_, reject) =>
          init?.signal?.addEventListener("abort", () => reject(new Error("aborted"))),
        ),
    );

    await expect(
      exchangeCode({
        issuer: ISSUER,
        clientId: "c",
        clientSecret: "s",
        code: "x",
        codeVerifier: "v",
        fetch: hanging as unknown as typeof fetch,
        timeoutMs: 5,
      }),
    ).rejects.toThrow(/aborted/);
  });
});

describe("revokeToken and userinfo", () => {
  it("revokes with client authentication and resolves on 200", async () => {
    const { fetch, first } = fakeFetch(() => new Response(null, { status: 200 }));

    await revokeToken({
      issuer: ISSUER,
      clientId: "c",
      clientSecret: "s",
      token: "ebk_key",
      fetch,
    });

    expect(first().url).toBe(ISSUER + "/oauth/revoke");
    expect(first().init.body).toBe("token=ebk_key&token_type_hint=access_token");
    expect((first().init.headers as Record<string, string>).Authorization).toMatch(/^Basic /);
  });

  it("throws invalid_client on a bad secret", async () => {
    const { fetch } = fakeFetch(() =>
      jsonResponse(401, { error: "invalid_client" }, { "WWW-Authenticate": 'Basic realm="oauth"' }),
    );

    await expect(
      revokeToken({ issuer: ISSUER, clientId: "c", clientSecret: "bad", token: "t", fetch }),
    ).rejects.toMatchObject({
      status: 401,
      error: "invalid_client",
    });
  });

  it("reads userinfo with a Bearer key", async () => {
    const { fetch, first } = fakeFetch(() =>
      jsonResponse(200, {
        sub: "wl:p:connect:a@b",
        email: "a@b",
        partner_id: "p",
        client_id: "c",
        scope: "accounts.read",
        expires_at: null,
      }),
    );

    const info = await userinfo({ issuer: ISSUER, accessToken: "ebk_key", fetch });

    expect(first().url).toBe(ISSUER + "/oauth/userinfo");
    expect((first().init.headers as Record<string, string>).Authorization).toBe("Bearer ebk_key");
    expect(info).toEqual({
      sub: "wl:p:connect:a@b",
      email: "a@b",
      partnerId: "p",
      clientId: "c",
      scope: "accounts.read",
      expiresAt: null,
    });
  });

  it("surfaces a revoked key as a 401 OAuthError", async () => {
    const { fetch } = fakeFetch(() => new Response("", { status: 401 }));

    await expect(
      userinfo({ issuer: ISSUER, accessToken: "ebk_gone", fetch }),
    ).rejects.toMatchObject({ status: 401 });
  });
});

describe("discover", () => {
  const metadata = {
    issuer: ISSUER,
    authorization_endpoint: ISSUER + "/oauth/authorize",
    token_endpoint: ISSUER + "/oauth/token",
    revocation_endpoint: ISSUER + "/oauth/revoke",
    scopes_supported: ["accounts.read"],
    response_types_supported: ["code"],
    response_modes_supported: ["form_post"],
    grant_types_supported: ["authorization_code"],
    token_endpoint_auth_methods_supported: ["client_secret_post", "client_secret_basic"],
    revocation_endpoint_auth_methods_supported: ["client_secret_post", "client_secret_basic"],
    code_challenge_methods_supported: ["S256"],
    authorization_response_iss_parameter_supported: true,
    open_banking_io: {
      userinfo_endpoint: ISSUER + "/oauth/userinfo",
      api_base_url: ISSUER,
      api_key_header: "X-Api-Key",
      bearer_supported: true,
      login_challenges_supported: ["pin_code"],
      key_relay: { response_mode: "form_post", fields: [...CONNECT_RELAY_FIELDS] },
      documentation: "https://open-banking.io/en/docs/partners",
    },
  };

  it("fetches the well-known document and caches it per issuer", async () => {
    const { fetch, calls, first } = fakeFetch(() => jsonResponse(200, metadata));

    const one = await discover(ISSUER, { fetch });
    const two = await discover(ISSUER + "/", { fetch });

    expect(calls).toHaveLength(1);
    expect(first().url).toBe(ISSUER + "/.well-known/oauth-authorization-server");
    expect(one.open_banking_io.key_relay.fields).toEqual([...CONNECT_RELAY_FIELDS]);
    expect(two).toBe(one);
  });

  it("rejects a document whose issuer does not match", async () => {
    const { fetch } = fakeFetch(() =>
      jsonResponse(200, { ...metadata, issuer: "https://evil.example" }),
    );

    await expect(discover("https://mismatch.example", { fetch })).rejects.toThrow(
      /Issuer mismatch/,
    );
  });
});

describe("bundleFromToken and OpenBankingClient.fromTokenResponse", () => {
  const token = {
    accessToken: "ebk_key",
    tokenType: "Bearer",
    expiresIn: 1,
    scope: "accounts.read",
    apiKey: "ebk_key",
    apiBaseUrl: ISSUER,
    user: "wl:p:connect:a@b",
  };

  it("builds the credentials bundle the data client reads with", () => {
    expect(bundleFromToken(token, "pk")).toEqual({
      service: "open-banking.io",
      apiBaseUrl: ISSUER,
      user: "wl:p:connect:a@b",
      apiKey: "ebk_key",
      encryptionKey: { scheme: "ecdh-p256-hkdf-aes-256-gcm", curve: "P-256", privateKey: "pk" },
    });
  });

  it("constructs a client straight from the token response", () => {
    const client = OpenBankingClient.fromTokenResponse(token, PRIVATE_KEY);
    expect(client).toBeInstanceOf(OpenBankingClient);
  });
});
