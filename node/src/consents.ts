import { OAuthError, revokeToken, userinfo, type HttpOptions } from "./connect.js";
import { decryptTo, importPrivateKey } from "./envelope.js";
import type { OpenConsentWire, SessionIdEnc } from "./models.js";
import { USER_AGENT } from "./version.js";

export interface CloseReplacedConsentsOptions extends HttpOptions {
  issuer: string;
  clientId: string;
  clientSecret: string;
  /** The PREVIOUS key — the one issued before the renewal. It is revoked by this call. */
  token: string;
  /** The base64 PKCS#8 private key the consents are sealed to. */
  privateKey: string;
  /** Defaults to the issuer. */
  apiBaseUrl?: string;
}

export interface CloseReplacedConsentsResult {
  /** Consents sent to be closed at the bank. */
  closed: number;
  /** Consents listed but not closable (no envelope, or it would not decrypt). */
  failed: number;
  /** Whether the previous key is gone. When false, revoke it yourself. */
  revoked: boolean;
}

/**
 * After a renewal: closes, at the bank, the consents the previous key still has open, and revokes
 * that key in the same call. Reads `GET /api/connections/open-consents` only — never the live
 * connections, which by now hold the renewal. A key that is already gone, or was issued to another
 * client, closes nothing: the service answers such a revoke with 200 without closing anything. A
 * failure that retrying may clear (5xx, 429) throws, so the key survives for a retry: revoking it
 * without the close would leave the consents open with nothing left that can list them.
 */
export async function closeReplacedConsents(
  options: CloseReplacedConsentsOptions,
): Promise<CloseReplacedConsentsResult> {
  const fetchImpl = options.fetch ?? fetch;
  const base = trimSlash(options.apiBaseUrl ?? options.issuer);
  const nothing = { closed: 0, failed: 0, revoked: false };

  let owner: string | null;
  try {
    owner = (await userinfo({ ...options, accessToken: options.token })).clientId;
  } catch (e) {
    if (e instanceof OAuthError && keyRefused(e.status)) return nothing;
    throw e;
  }
  if (owner !== options.clientId) return nothing;

  const res = await fetchImpl(`${base}/api/connections/open-consents`, {
    headers: { "X-Api-Key": options.token, "User-Agent": USER_AGENT },
    signal: AbortSignal.timeout(options.timeoutMs ?? 30_000),
  });
  if (keyRefused(res.status)) return nothing;
  if (!res.ok) throw new Error(`GET /api/connections/open-consents failed: ${res.status}`);
  const open = (await res.json()) as OpenConsentWire[];

  const key = await importPrivateKey(options.privateKey);
  const closeConsents: { connectionId: string; ebSessionId: string }[] = [];
  const seen = new Set<string>();
  let failed = 0;
  for (const c of open) {
    if (!c.connectionId || seen.has(c.connectionId)) continue;
    seen.add(c.connectionId);
    if (!c.sessionIdEnc) {
      failed += 1;
      continue;
    }
    let sessionId: string | null | undefined;
    try {
      sessionId = (await decryptTo<SessionIdEnc>(key, c.sessionIdEnc))?.sessionId;
    } catch {
      sessionId = null;
    }
    if (!sessionId) {
      failed += 1;
      continue;
    }
    closeConsents.push({ connectionId: c.connectionId, ebSessionId: sessionId });
  }

  try {
    await revokeToken({ ...options, closeConsents });
  } catch (e) {
    if (e instanceof OAuthError && e.status < 500 && e.status !== 429)
      return { closed: 0, failed: failed + closeConsents.length, revoked: false };
    throw e;
  }
  return { closed: closeConsents.length, failed, revoked: true };
}

const keyRefused = (status: number) => status === 401 || status === 403;

function trimSlash(url: string): string {
  let end = url.length;
  while (end > 0 && url.charCodeAt(end - 1) === 47) end--;
  return url.slice(0, end);
}
