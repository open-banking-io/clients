import { bundleFromToken, type TokenResponse } from "./connect.js";
import { readFileSync } from "node:fs";
import { decryptTo, importPrivateKey, type CryptoKey } from "./envelope.js";
import { SyncError } from "./errors.js";
import { USER_AGENT } from "./version.js";
import type {
  Account,
  AccountEnc,
  AccountWire,
  BalanceEnc,
  Connection,
  ConnectionWire,
  CredentialsBundle,
  DisplayNameEnc,
  OpenBankingClientOptions,
  PsuHeaders,
  SyncAllResult,
  SyncAllResultWire,
  SyncFailure,
  SyncOptions,
  SyncResult,
  SyncResultWire,
  Transaction,
  TransactionEnc,
  TransactionPage,
  TransactionPageWire,
  TransactionQuery,
  TransactionWire,
  UidEnc,
} from "./models.js";

/**
 * Server-to-server client for open-banking.io. Authenticates with an API key (`X-Api-Key`) and
 * decrypts the zero-knowledge data envelopes locally with the exported private key — the service
 * only ever returns ciphertext it cannot read.
 */
export class OpenBankingClient {
  /** Default per-request timeout (30s) when {@link OpenBankingClientOptions.timeoutMs} is unset. */
  static readonly DEFAULT_TIMEOUT_MS = 30_000;

  private readonly baseUrl: string;
  private readonly apiKey: string;
  private readonly timeoutMs: number;
  private readonly fetchImpl: typeof globalThis.fetch;
  private readonly privateKey: Promise<CryptoKey>;

  constructor(options: OpenBankingClientOptions) {
    const { apiBaseUrl, apiKey, privateKeyPkcs8, timeoutMs } = options;
    if (!apiBaseUrl?.trim()) throw new Error("apiBaseUrl is required");
    if (!apiKey?.trim()) throw new Error("apiKey is required");
    if (!privateKeyPkcs8?.trim()) throw new Error("privateKeyPkcs8 is required");

    // Strip all trailing "/" without a backtracking regex (avoids ReDoS on long inputs).
    let end = apiBaseUrl.length;
    while (end > 0 && apiBaseUrl.charCodeAt(end - 1) === 47 /* "/" */) end--;
    this.baseUrl = apiBaseUrl.slice(0, end);
    this.apiKey = apiKey;
    this.timeoutMs = timeoutMs ?? OpenBankingClient.DEFAULT_TIMEOUT_MS;
    this.fetchImpl = options.fetch ?? fetch;
    this.privateKey = importPrivateKey(privateKeyPkcs8);
  }

  /**
   * Builds a client from a credentials-bundle JSON string or a path to a bundle file.
   *
   * `overrides` (optional) lets the bundle path set the same transport knobs as the
   * constructor — e.g. a custom {@link OpenBankingClientOptions.fetch} or
   * {@link OpenBankingClientOptions.timeoutMs}.
   */
  static fromCredentials(
    jsonOrPath: string,
    overrides: Partial<Pick<OpenBankingClientOptions, "fetch" | "timeoutMs">> = {},
  ): OpenBankingClient {
    let json = jsonOrPath;
    if (!jsonOrPath.trimStart().startsWith("{")) {
      json = readFileSync(jsonOrPath, "utf8");
    }
    const bundle = JSON.parse(json) as CredentialsBundle;
    return OpenBankingClient.fromBundle(bundle, overrides);
  }

  static fromBundle(
    bundle: CredentialsBundle,
    overrides: Partial<Pick<OpenBankingClientOptions, "fetch" | "timeoutMs">> = {},
  ): OpenBankingClient {
    if (!bundle.apiKey?.trim()) throw new Error("The credentials bundle has no apiKey");
    return new OpenBankingClient({
      apiBaseUrl: bundle.apiBaseUrl,
      apiKey: bundle.apiKey,
      privateKeyPkcs8: bundle.encryptionKey.privateKey,
      timeoutMs: overrides.timeoutMs,
      fetch: overrides.fetch,
    });
  }

  /**
   * Builds a client from a Partner Connect token response and the private key the consent page
   * relayed — see {@link exchangeCode} and {@link parseRelay}.
   */
  static fromTokenResponse(
    token: TokenResponse,
    privateKey: string,
    overrides: Partial<Pick<OpenBankingClientOptions, "fetch" | "timeoutMs">> = {},
  ): OpenBankingClient {
    return OpenBankingClient.fromBundle(bundleFromToken(token, privateKey), overrides);
  }

  /** Lists the user's accounts with all sensitive fields decrypted. */
  async getAccounts(): Promise<Account[]> {
    const wires = await this.getAccountWires();
    return Promise.all(wires.map((w) => this.mapAccount(w)));
  }

  /** Returns a page of an account's statement, newest first, with decrypted fields. */
  async getTransactions(accountId: string, query: TransactionQuery = {}): Promise<TransactionPage> {
    const qs = new URLSearchParams();
    if (query.from != null) qs.set("from", query.from);
    if (query.to != null) qs.set("to", query.to);
    if (query.limit != null) qs.set("limit", String(query.limit));
    if (query.offset != null) qs.set("offset", String(query.offset));
    const suffix = qs.toString() ? `?${qs.toString()}` : "";

    const page = await this.getJson<TransactionPageWire>(
      `/api/accounts/${encodeURIComponent(accountId)}/transactions${suffix}`,
    );

    const items = await Promise.all((page.items ?? []).map((t) => this.mapTransaction(t)));
    return { items, total: page.total };
  }

  /** Lists the user's bank connections. */
  async getConnections(): Promise<Connection[]> {
    const wires = await this.getJson<ConnectionWire[]>("/api/connections");
    return wires.map((c) => ({
      sessionId: c.sessionId,
      aspspName: c.aspspName,
      aspspCountry: c.aspspCountry,
      validUntil: c.validUntil,
      status: c.status,
      accountCount: c.accountCount,
      lastSyncedAt: c.lastSyncedAt ?? null,
      psuType: c.psuType ?? null,
      isLive: c.isLive ?? (c.status === "Active" && Date.parse(c.validUntil) > Date.now()),
      accountIds: c.accountIds ?? [],
    }));
  }

  /**
   * Triggers an online sync of one account: decrypts that account's Enable Banking uid and posts
   * it, so the service can fetch fresh data without ever holding the uid in plaintext.
   *
   * A refusal throws {@link SyncError}. A uid a renewal replaced (`uid_outdated`) is re-read and
   * retried once.
   */
  async sync(accountId: string, options: SyncOptions = {}): Promise<SyncResult> {
    for (let attempt = 0; ; attempt++) {
      const wires = await this.getAccountWires();
      const account = wires.find((a) => a.id === accountId);
      if (!account) throw new Error(`Account ${accountId} not found`);

      const uid = await this.decryptUid(account);
      if (uid == null) {
        throw new SyncError(
          "Account has no active session (reconnect required) — cannot sync",
          409,
          "reconnect_needed",
        );
      }

      try {
        const result = await this.postJson<SyncResultWire>(
          `/api/accounts/${encodeURIComponent(accountId)}/sync`,
          { uid },
          psuHeaders(options.psu),
        );
        return { newTransactions: result.newTransactions, totalFetched: result.totalFetched };
      } catch (e) {
        if (attempt === 0 && e instanceof SyncError && e.reason === "uid_outdated") continue;
        throw e;
      }
    }
  }

  /**
   * Triggers an online sync of every account that has an active session. Per-account refusals are
   * in `failures`; accounts answering `uid_outdated` are re-read and retried once.
   */
  async syncAll(options: SyncOptions = {}): Promise<SyncAllResult> {
    const first = await this.postSyncAll(await this.syncItems(), options);
    const outdated = new Set(
      first.failures.filter((f) => f.reason === "uid_outdated").map((f) => f.accountId),
    );
    if (outdated.size === 0) return first;

    const retryItems = (await this.syncItems()).filter((i) => outdated.has(i.accountId));
    if (retryItems.length === 0) return first;

    const second = await this.postSyncAll(retryItems, options);
    const retried = new Set(retryItems.map((i) => i.accountId));
    return {
      accounts: first.accounts + second.accounts,
      newTransactions: first.newTransactions + second.newTransactions,
      failures: [...first.failures.filter((f) => !retried.has(f.accountId)), ...second.failures],
    };
  }

  // ---- internals ---------------------------------------------------------------------------------

  private getAccountWires(): Promise<AccountWire[]> {
    return this.getJson<AccountWire[]>("/api/accounts");
  }

  private async syncItems(): Promise<{ accountId: string; uid: string }[]> {
    const wires = await this.getAccountWires();
    const decrypted = await Promise.all(
      wires.map(async (a) => ({ accountId: a.id, uid: await this.decryptUid(a) })),
    );
    return decrypted
      .filter((x): x is { accountId: string; uid: string } => x.uid != null)
      .map((x) => ({ accountId: x.accountId, uid: x.uid }));
  }

  private async postSyncAll(
    items: { accountId: string; uid: string }[],
    options: SyncOptions,
  ): Promise<SyncAllResult> {
    const result = await this.postJson<SyncAllResultWire>(
      "/api/sync",
      { items },
      psuHeaders(options.psu),
    );
    const failures: SyncFailure[] = (result.failures ?? []).map((f) => ({
      accountId: f.accountId,
      reason: f.reason,
      bankErrorCode: f.bankErrorCode ?? null,
    }));
    return { accounts: result.accounts, newTransactions: result.newTransactions, failures };
  }

  private async decryptUid(a: AccountWire): Promise<string | null> {
    const dec = await decryptTo<UidEnc>(await this.privateKey, a.uidEnc);
    return dec?.uid ?? null;
  }

  private async mapAccount(a: AccountWire): Promise<Account> {
    const priv = await this.privateKey;
    const acc = await decryptTo<AccountEnc>(priv, a.enc);
    const name = await decryptTo<DisplayNameEnc>(priv, a.displayNameEnc);
    const balances = await Promise.all(
      (a.balances ?? []).map(async (b) => {
        const dec = await decryptTo<BalanceEnc>(priv, b.enc);
        return {
          type: b.type,
          name: dec?.name ?? null,
          amount: dec?.amount ?? "0",
          currency: b.currency,
          referenceDate: b.referenceDate ?? null,
        };
      }),
    );

    return {
      id: a.id,
      aspspName: a.aspspName,
      aspspCountry: a.aspspCountry,
      currency: a.currency,
      accountType: a.accountType ?? null,
      bic: a.bic ?? null,
      needsReconnect: a.needsReconnect,
      iban: acc?.iban ?? null,
      bban: acc?.bban ?? null,
      ownerName: acc?.ownerName ?? null,
      accountName: acc?.accountName ?? null,
      product: acc?.product ?? null,
      displayName: name?.displayName ?? null,
      balances,
    };
  }

  private async mapTransaction(t: TransactionWire): Promise<Transaction> {
    const d = await decryptTo<TransactionEnc>(await this.privateKey, t.enc);
    return {
      id: t.id,
      currency: t.currency,
      creditDebitIndicator: t.creditDebitIndicator,
      status: t.status ?? null,
      bookingDate: t.bookingDate ?? null,
      valueDate: t.valueDate ?? null,
      transactionDate: t.transactionDate ?? null,
      bankTransactionCode: t.bankTransactionCode ?? null,
      amount: d?.amount ?? "0",
      creditorName: d?.creditorName ?? null,
      creditorIban: d?.creditorIban ?? null,
      creditorBban: d?.creditorBban ?? null,
      creditorAgentBic: d?.creditorAgentBic ?? null,
      debtorName: d?.debtorName ?? null,
      debtorIban: d?.debtorIban ?? null,
      debtorBban: d?.debtorBban ?? null,
      debtorAgentBic: d?.debtorAgentBic ?? null,
      remittanceInformation: d?.remittanceInformation ?? null,
      note: d?.note ?? null,
      referenceNumber: d?.referenceNumber ?? null,
      exchangeRate: d?.exchangeRate ?? null,
      merchantCategoryCode: d?.merchantCategoryCode ?? null,
      balanceAfterTransaction: d?.balanceAfter ?? null,
      balanceAfterCurrency: d?.balanceAfterCurrency ?? null,
    };
  }

  private async getJson<T>(path: string): Promise<T> {
    const res = await this.fetchImpl(this.baseUrl + path, {
      headers: { "X-Api-Key": this.apiKey, "User-Agent": USER_AGENT },
      signal: AbortSignal.timeout(this.timeoutMs),
    });
    if (!res.ok) {
      throw new Error(`GET ${path} failed: ${res.status} ${res.statusText}`);
    }
    return (await res.json()) as T;
  }

  private async postJson<T>(
    path: string,
    body: unknown,
    extraHeaders: Record<string, string> = {},
  ): Promise<T> {
    const res = await this.fetchImpl(this.baseUrl + path, {
      method: "POST",
      headers: {
        ...extraHeaders,
        "X-Api-Key": this.apiKey,
        "Content-Type": "application/json",
        "User-Agent": USER_AGENT,
      },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(this.timeoutMs),
    });
    if (!res.ok) {
      const problem = await readProblem(res);
      const retryAfter = Number.parseInt(res.headers.get("Retry-After") ?? "", 10);
      throw new SyncError(
        `POST ${path} failed: ${res.status} ${problem.reason ?? res.statusText}`,
        res.status,
        problem.reason,
        problem.bankErrorCode,
        Number.isFinite(retryAfter) ? retryAfter : null,
      );
    }
    return (await res.json()) as T;
  }
}

async function readProblem(
  res: Response,
): Promise<{ reason: string | null; bankErrorCode: string | null }> {
  try {
    const body = (await res.json()) as { reason?: unknown; bankErrorCode?: unknown };
    return {
      reason: typeof body.reason === "string" ? body.reason : null,
      bankErrorCode: typeof body.bankErrorCode === "string" ? body.bankErrorCode : null,
    };
  } catch {
    return { reason: null, bankErrorCode: null };
  }
}

/** The `X-Psu-*` request headers for a forwarded {@link PsuHeaders}; empty when none is given. */
export function psuHeaders(psu: PsuHeaders | undefined): Record<string, string> {
  if (!psu) return {};
  const headers: Record<string, string> = {
    "X-Psu-Ip-Address": psu.ipAddress,
    "X-Psu-User-Agent": psu.userAgent,
  };
  const optional: [string, string | undefined][] = [
    ["X-Psu-Referer", psu.referer],
    ["X-Psu-Accept", psu.accept],
    ["X-Psu-Accept-Language", psu.acceptLanguage],
    ["X-Psu-Accept-Charset", psu.acceptCharset],
    ["X-Psu-Accept-Encoding", psu.acceptEncoding],
  ];
  for (const [name, value] of optional) if (value) headers[name] = value;
  return headers;
}
