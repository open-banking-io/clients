// Partner Connect: the OAuth 2.0 authorization-code flow (PKCE, form_post) that hands a partner an
// API key and the user's private key. Documented at https://open-banking.io/en/docs/partners.
import { createHash, randomBytes, timingSafeEqual } from "node:crypto";
import type { CredentialsBundle } from "./models.js";
import { USER_AGENT } from "./version.js";

const DEFAULT_TIMEOUT_MS = 30_000;

/** The relay fields the consent page form-posts to a redirect URI, in wire order. */
export const CONNECT_RELAY_FIELDS = ["code", "state", "iss", "privateKey", "publicKey"] as const;

export interface Pkce {
  /** The `code_verifier` to keep server-side and send on the token request. */
  verifier: string;
  /** The `code_challenge` to send on the authorize request. */
  challenge: string;
  method: "S256";
}

/** A fresh PKCE pair (RFC 7636): 32 random bytes, base64url, S256 challenge. */
export function createPkce(): Pkce {
  const verifier = randomBytes(32).toString("base64url");
  return { verifier, challenge: pkceChallenge(verifier), method: "S256" };
}

/** The S256 challenge for a verifier — exposed so a stored verifier can be re-checked. */
export function pkceChallenge(verifier: string): string {
  return createHash("sha256").update(verifier).digest("base64url");
}

/** A fresh opaque `state` (16 random bytes, base64url). */
export function createState(): string {
  return randomBytes(16).toString("base64url");
}

export interface AuthorizeUrlOptions {
  /** The issuer, e.g. `https://open-banking.io`. */
  issuer: string;
  clientId: string;
  /** One of the client's registered redirect URIs, character for character. */
  redirectUri: string;
  state: string;
  /** From {@link createPkce}. */
  codeChallenge: string;
  /** Defaults to `accounts.read`, the only scope. */
  scope?: string;
  /** Prefills the email on the login page; never authenticates. */
  loginHint?: string;
  /** `pin_code` keeps a popup journey inside the popup (typed emailed code instead of a magic link). */
  challenge?: "pin_code";
}

/** The `/oauth/authorize` URL for a top-level browser navigation. */
export function buildAuthorizeUrl(options: AuthorizeUrlOptions): string {
  const params = new URLSearchParams({
    response_type: "code",
    client_id: options.clientId,
    redirect_uri: options.redirectUri,
    scope: options.scope ?? "accounts.read",
    state: options.state,
    code_challenge: options.codeChallenge,
    code_challenge_method: "S256",
  });
  if (options.loginHint) params.set("login_hint", options.loginHint);
  if (options.challenge) params.set("challenge", options.challenge);
  return `${endpoint(options.issuer, "/oauth/authorize")}?${params.toString()}`;
}

/** What the consent page relayed to the redirect URI. */
export interface ConnectRelay {
  code: string;
  state: string;
  /** Present on current servers (RFC 9207); empty string from older ones. */
  iss: string;
  /** The user's base64 PKCS#8 private key. A credential: never log it. */
  privateKey: string;
  publicKey: string;
}

export type RelayErrorCode =
  "oauth_error" | "state_mismatch" | "issuer_mismatch" | "missing_code" | "missing_private_key";

/** Thrown by {@link parseRelay}. `code` says why; for `oauth_error` the server's fields are attached. */
export class RelayError extends Error {
  readonly code: RelayErrorCode;
  readonly error?: string;
  readonly errorDescription?: string;

  constructor(
    code: RelayErrorCode,
    message: string,
    oauth?: { error: string; errorDescription?: string },
  ) {
    super(message);
    this.name = "RelayError";
    this.code = code;
    this.error = oauth?.error;
    this.errorDescription = oauth?.errorDescription;
  }
}

export interface ParseRelayOptions {
  /** The `state` this flow was started with. Compared in constant time. */
  expectedState: string;
  /** When set, `iss` must equal it (trailing slashes ignored). */
  issuer?: string;
  /** Require `iss` to be present even when `issuer` is not checked. Default: only when `issuer` is set. */
  requireIss?: boolean;
}

/** A form body, `URLSearchParams`, parsed body object, or JSON string. */
export type RelayInput = string | URLSearchParams | FormData | Record<string, unknown>;

/**
 * Validates the form_post relay: an OAuth error becomes a {@link RelayError} (`oauth_error`); then
 * `state` (constant time), `iss`, `code` and `privateKey` are checked in that order.
 */
export function parseRelay(input: RelayInput, options: ParseRelayOptions): ConnectRelay {
  const fields = toRecord(input);
  const read = (name: string): string => {
    const value = fields[name];
    return typeof value === "string" ? value : "";
  };

  const error = read("error");
  if (error) {
    const description = read("error_description");
    throw new RelayError("oauth_error", description ? `${error}: ${description}` : error, {
      error,
      errorDescription: description || undefined,
    });
  }

  if (!options.expectedState || !constantTimeEqual(read("state"), options.expectedState)) {
    throw new RelayError("state_mismatch", "The relayed state does not match this flow");
  }

  const iss = read("iss");
  const wantIss = options.issuer !== undefined || options.requireIss === true;
  if (wantIss && !iss) throw new RelayError("issuer_mismatch", "The relay carries no iss");
  if (
    options.issuer !== undefined &&
    !constantTimeEqual(trimSlash(iss), trimSlash(options.issuer))
  ) {
    throw new RelayError("issuer_mismatch", "The relayed iss is not the expected issuer");
  }

  const code = read("code");
  if (!code) throw new RelayError("missing_code", "The relay carries no authorization code");
  const privateKey = read("privateKey");
  if (!privateKey) throw new RelayError("missing_private_key", "The relay carries no private key");

  return { code, state: read("state"), iss, privateKey, publicKey: read("publicKey") };
}

/** An RFC 6749 §5.2 error body from the token or revocation endpoint. */
export class OAuthError extends Error {
  readonly status: number;
  readonly error: string;
  readonly errorDescription?: string;

  constructor(status: number, error: string, errorDescription?: string) {
    super(errorDescription ? `${error}: ${errorDescription}` : error);
    this.name = "OAuthError";
    this.status = status;
    this.error = error;
    this.errorDescription = errorDescription;
  }
}

interface HttpOptions {
  /** Custom `fetch`, e.g. for tests. Defaults to the global. */
  fetch?: typeof globalThis.fetch;
  /** Per-request timeout; defaults to 30s. */
  timeoutMs?: number;
}

export interface ExchangeCodeOptions extends HttpOptions {
  issuer: string;
  clientId: string;
  clientSecret: string;
  /** From the relay. */
  code: string;
  /** From {@link createPkce}, kept server-side. */
  codeVerifier: string;
  /** The redirect URI used on the authorize request; sent when given and must match. */
  redirectUri?: string;
}

/** The token endpoint's response. `accessToken` and `apiKey` are the same key. */
export interface TokenResponse {
  accessToken: string;
  tokenType: string;
  expiresIn: number;
  scope: string;
  apiKey: string;
  apiBaseUrl: string;
  /** The tenant subject the key reads for. */
  user: string;
}

interface TokenResponseWire {
  access_token?: string;
  token_type?: string;
  expires_in?: number;
  scope?: string;
  apiKey?: string;
  apiBaseUrl?: string;
  user?: string;
}

/** Exchanges the relayed code for the key, server to server, with HTTP Basic client authentication. */
export async function exchangeCode(options: ExchangeCodeOptions): Promise<TokenResponse> {
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code: options.code,
    code_verifier: options.codeVerifier,
  });
  if (options.redirectUri) body.set("redirect_uri", options.redirectUri);

  const response = await post(
    endpoint(options.issuer, "/oauth/token"),
    body,
    {
      Authorization: basicAuth(options.clientId, options.clientSecret),
      Accept: "application/json",
    },
    options,
  );
  await throwIfOAuthError(response);

  const wire = (await response.json()) as TokenResponseWire;
  const accessToken = wire.access_token ?? wire.apiKey;
  if (!accessToken)
    throw new OAuthError(
      response.status,
      "invalid_response",
      "The token response carries no access token",
    );
  return {
    accessToken,
    tokenType: wire.token_type ?? "Bearer",
    expiresIn: wire.expires_in ?? 0,
    scope: wire.scope ?? "",
    apiKey: wire.apiKey ?? accessToken,
    apiBaseUrl: wire.apiBaseUrl ?? trimSlash(options.issuer),
    user: wire.user ?? "",
  };
}

export interface RevokeTokenOptions extends HttpOptions {
  issuer: string;
  clientId: string;
  clientSecret: string;
  /** The key to revoke. Only keys issued to this client are touched; an unknown key still succeeds. */
  token: string;
}

/** RFC 7009 revocation. Resolves on 200; throws {@link OAuthError} on a client-authentication failure. */
export async function revokeToken(options: RevokeTokenOptions): Promise<void> {
  const body = new URLSearchParams({ token: options.token, token_type_hint: "access_token" });
  const response = await post(
    endpoint(options.issuer, "/oauth/revoke"),
    body,
    {
      Authorization: basicAuth(options.clientId, options.clientSecret),
    },
    options,
  );
  await throwIfOAuthError(response);
}

export interface UserinfoOptions extends HttpOptions {
  issuer: string;
  accessToken: string;
}

export interface Userinfo {
  sub: string;
  email: string;
  partnerId: string | null;
  clientId: string | null;
  scope: string;
  expiresAt: string | null;
}

interface UserinfoWire {
  sub: string;
  email: string;
  partner_id: string | null;
  client_id: string | null;
  scope: string;
  expires_at: string | null;
}

/** What a key reads for. Throws {@link OAuthError} with status 401 when the key is revoked or expired. */
export async function userinfo(options: UserinfoOptions): Promise<Userinfo> {
  const response = await request(
    endpoint(options.issuer, "/oauth/userinfo"),
    {
      method: "GET",
      headers: {
        Authorization: `Bearer ${options.accessToken}`,
        Accept: "application/json",
        "User-Agent": USER_AGENT,
      },
    },
    options,
  );
  await throwIfOAuthError(response);
  const wire = (await response.json()) as UserinfoWire;
  return {
    sub: wire.sub,
    email: wire.email,
    partnerId: wire.partner_id ?? null,
    clientId: wire.client_id ?? null,
    scope: wire.scope,
    expiresAt: wire.expires_at ?? null,
  };
}

/** RFC 8414 metadata with the `open_banking_io` extension. */
export interface ServerMetadata {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  revocation_endpoint: string;
  scopes_supported: string[];
  response_types_supported: string[];
  response_modes_supported: string[];
  grant_types_supported: string[];
  token_endpoint_auth_methods_supported: string[];
  code_challenge_methods_supported: string[];
  service_documentation?: string;
  open_banking_io: {
    userinfo_endpoint: string;
    api_base_url: string;
    api_key_header: string;
    bearer_supported: boolean;
    key_relay: { response_mode: string; fields: string[] };
    documentation: string;
  };
}

const discoveryCache = new Map<string, { metadata: ServerMetadata; expiresAt: number }>();
const DISCOVERY_TTL_MS = 60 * 60 * 1000;

/** Fetches `/.well-known/oauth-authorization-server`, cached per issuer for an hour. */
export async function discover(issuer: string, options: HttpOptions = {}): Promise<ServerMetadata> {
  const key = trimSlash(issuer);
  const cached = discoveryCache.get(key);
  if (cached && cached.expiresAt > Date.now()) return cached.metadata;

  const response = await request(
    `${key}/.well-known/oauth-authorization-server`,
    {
      method: "GET",
      headers: { Accept: "application/json", "User-Agent": USER_AGENT },
    },
    options,
  );
  if (!response.ok)
    throw new OAuthError(response.status, "discovery_failed", `HTTP ${response.status}`);
  const metadata = (await response.json()) as ServerMetadata;
  if (trimSlash(metadata.issuer) !== key) {
    throw new OAuthError(
      response.status,
      "discovery_failed",
      `Issuer mismatch: ${metadata.issuer}`,
    );
  }
  discoveryCache.set(key, { metadata, expiresAt: Date.now() + DISCOVERY_TTL_MS });
  return metadata;
}

/** The credentials bundle the data client reads with: the key plus the relayed private key. */
export function bundleFromToken(token: TokenResponse, privateKey: string): CredentialsBundle {
  return {
    service: "open-banking.io",
    apiBaseUrl: token.apiBaseUrl,
    user: token.user,
    apiKey: token.accessToken,
    encryptionKey: { scheme: "ecdh-p256-hkdf-aes-256-gcm", curve: "P-256", privateKey },
  };
}

function endpoint(issuer: string, path: string): string {
  return trimSlash(issuer) + path;
}

function trimSlash(value: string): string {
  let end = value.length;
  while (end > 0 && value[end - 1] === "/") end--;
  return value.slice(0, end);
}

function basicAuth(clientId: string, clientSecret: string): string {
  const pair = `${encodeURIComponent(clientId)}:${encodeURIComponent(clientSecret)}`;
  return `Basic ${Buffer.from(pair, "utf8").toString("base64")}`;
}

function constantTimeEqual(a: string, b: string): boolean {
  const x = createHash("sha256").update(a, "utf8").digest();
  const y = createHash("sha256").update(b, "utf8").digest();
  return timingSafeEqual(x, y) && a.length === b.length;
}

function toRecord(input: RelayInput): Record<string, unknown> {
  if (typeof input === "string") {
    const trimmed = input.trim();
    if (trimmed.startsWith("{")) return JSON.parse(trimmed) as Record<string, unknown>;
    return Object.fromEntries(new URLSearchParams(trimmed));
  }
  if (input instanceof URLSearchParams) return Object.fromEntries(input);
  if (input instanceof FormData) {
    const record: Record<string, unknown> = {};
    input.forEach((value, name) => {
      if (typeof value === "string") record[name] = value;
    });
    return record;
  }
  return input;
}

async function post(
  url: string,
  body: URLSearchParams,
  headers: Record<string, string>,
  options: HttpOptions,
): Promise<Response> {
  return request(
    url,
    {
      method: "POST",
      headers: {
        ...headers,
        "Content-Type": "application/x-www-form-urlencoded",
        "User-Agent": USER_AGENT,
      },
      body: body.toString(),
    },
    options,
  );
}

async function request(url: string, init: RequestInit, options: HttpOptions): Promise<Response> {
  const fetchImpl = options.fetch ?? fetch;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), options.timeoutMs ?? DEFAULT_TIMEOUT_MS);
  try {
    return await fetchImpl(url, { ...init, signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
}

async function throwIfOAuthError(response: Response): Promise<void> {
  if (response.ok) return;
  let error = "http_error";
  let description: string | undefined = `HTTP ${response.status}`;
  try {
    const body = (await response.json()) as { error?: string; error_description?: string };
    if (body.error) {
      error = body.error;
      description = body.error_description;
    }
  } catch {
    // A non-JSON error body keeps the HTTP status as the description.
  }
  throw new OAuthError(response.status, error, description);
}
