// ---- Public, decrypted models -------------------------------------------------------------------

/** A bank account with its sensitive fields decrypted. */
export interface Account {
  id: string;
  aspspName: string;
  aspspCountry: string;
  currency: string;
  accountType: string | null;
  bic: string | null;
  needsReconnect: boolean;

  // Decrypted from the account envelope.
  iban: string | null;
  bban: string | null;
  ownerName: string | null;
  accountName: string | null;
  product: string | null;
  displayName: string | null;

  balances: Balance[];
}

/** A balance snapshot. `type` is the ISO 20022 code (ITBD booked, ITAV available, …). */
export interface Balance {
  type: string;
  name: string | null;
  /** Decimal as a string — never parsed to float. */
  amount: string;
  currency: string;
  referenceDate: string | null;
}

/** A statement transaction with its sensitive fields decrypted. */
export interface Transaction {
  id: string;
  currency: string;
  creditDebitIndicator: string;
  status: string | null;
  bookingDate: string | null;
  valueDate: string | null;
  transactionDate: string | null;
  bankTransactionCode: string | null;

  /** Decimal as a string — never parsed to float. */
  amount: string;
  creditorName: string | null;
  creditorIban: string | null;
  creditorBban: string | null;
  creditorAgentBic: string | null;
  debtorName: string | null;
  debtorIban: string | null;
  debtorBban: string | null;
  debtorAgentBic: string | null;
  remittanceInformation: string | null;
  note: string | null;
  referenceNumber: string | null;
  exchangeRate: string | null;
  merchantCategoryCode: string | null;
  /** Decimal as a string, or `null`. */
  balanceAfterTransaction: string | null;
  balanceAfterCurrency: string | null;
}

/** A page of transactions, newest first. */
export interface TransactionPage {
  items: Transaction[];
  total: number;
}

/** A bank connection (consent). */
export interface Connection {
  sessionId: string;
  aspspName: string;
  aspspCountry: string;
  validUntil: string;
  status: string;
  accountCount: number;
  lastSyncedAt: string | null;
  psuType: string | null;
  /** Whether the consent still works, by the service's clock. `status` stays `Active` after `validUntil`. */
  isLive: boolean;
  /** The accounts this connection holds. */
  accountIds: string[];
}

export interface SyncResult {
  newTransactions: number;
  totalFetched: number;
}

/**
 * The stable reason a sync failed. Branch on this, never on the HTTP status.
 * A reason added later arrives as its raw string.
 */
export type SyncFailureReason =
  | "reconnect_needed"
  | "consent_withdrawn"
  | "uid_outdated"
  | "psu_present_required"
  | "rate_limited"
  | "bank_error"
  | "transient"
  | "partner_app_inactive"
  | (string & {});

export interface SyncFailure {
  accountId: string;
  reason: SyncFailureReason;
  bankErrorCode: string | null;
}

export interface SyncAllResult {
  accounts: number;
  newTransactions: number;
  /** The accounts that did not sync, and why. */
  failures: SyncFailure[];
}

/**
 * The account holder's own request, forwarded while they are on your page: some banks only share
 * data with the person present. Never send it from a background job. The service honours it only
 * from a key a Connect client issued, and only with a public IP address and a user agent — behind a
 * proxy, take the address from the header your proxy sets. Without both, nothing is sent.
 */
export interface PsuHeaders {
  /** The user's public IP address. */
  ipAddress: string;
  userAgent: string;
  referer?: string;
  accept?: string;
  acceptLanguage?: string;
  acceptCharset?: string;
  acceptEncoding?: string;
}

export interface SyncOptions {
  /** The present user's request details; see {@link PsuHeaders}. */
  psu?: PsuHeaders;
}

/** Options for {@link OpenBankingClient.getTransactions}. */
export interface TransactionQuery {
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

// ---- Credentials bundle -------------------------------------------------------------------------

export interface CredentialsBundle {
  service?: string;
  apiBaseUrl: string;
  user?: string;
  apiKey?: string;
  encryptionKey: EncryptionKey;
}

export interface EncryptionKey {
  scheme?: string;
  curve?: string;
  privateKeyFormat?: string;
  /** The PKCS#8 private key, base64-encoded. */
  privateKey: string;
  publicKey?: string;
}

export interface OpenBankingClientOptions {
  apiBaseUrl: string;
  apiKey: string;
  /** The base64 PKCS#8 encryption private key from your bundle. */
  privateKeyPkcs8: string;
  /**
   * Per-request timeout in milliseconds, so a hung connection can't block forever.
   * Defaults to {@link DEFAULT_TIMEOUT_MS} (30s).
   */
  timeoutMs?: number;
  /**
   * Custom `fetch` implementation, used in place of the global `fetch` for every request.
   * Typed to the WHATWG `fetch` signature only, so the SDK keeps zero runtime dependencies.
   *
   * A custom HTTP transport (proxy, custom CA/mTLS, keep-alive, connection pooling) is
   * expressed by passing a `fetch` bound to an undici `Dispatcher`, e.g.:
   *
   * ```ts
   * import { Agent, fetch as undiciFetch } from "undici";
   * const dispatcher = new Agent({ keepAliveTimeout: 60_000 });
   * const client = new OpenBankingClient({
   *   apiBaseUrl, apiKey, privateKeyPkcs8,
   *   fetch: (input, init) => undiciFetch(input, { ...init, dispatcher }),
   * });
   * ```
   *
   * Defaults to the global `fetch`.
   */
  fetch?: typeof globalThis.fetch;
}

// ---- Wire DTOs (what the API returns; sensitive fields are ciphertext) ---------------------------

export interface AccountWire {
  id: string;
  aspspName: string;
  aspspCountry: string;
  currency: string;
  accountType?: string | null;
  bic?: string | null;
  needsReconnect: boolean;
  balances: BalanceWire[];
  enc?: string | null;
  displayNameEnc?: string | null;
  uidEnc?: string | null;
}

export interface BalanceWire {
  type: string;
  currency: string;
  referenceDate?: string | null;
  enc?: string | null;
}

export interface TransactionPageWire {
  items: TransactionWire[];
  total: number;
}

export interface TransactionWire {
  id: string;
  currency: string;
  creditDebitIndicator: string;
  status?: string | null;
  bookingDate?: string | null;
  valueDate?: string | null;
  transactionDate?: string | null;
  bankTransactionCode?: string | null;
  enc?: string | null;
}

export interface ConnectionWire {
  sessionId: string;
  aspspName: string;
  aspspCountry: string;
  validUntil: string;
  status: string;
  accountCount: number;
  lastSyncedAt?: string | null;
  psuType?: string | null;
  isLive?: boolean;
  accountIds?: string[];
}

export interface SyncResultWire {
  newTransactions: number;
  totalFetched: number;
}

export interface SyncAllResultWire {
  accounts: number;
  newTransactions: number;
  failures?: { accountId: string; reason: string; bankErrorCode?: string | null }[];
}

export interface OpenConsentWire {
  connectionId: string;
  aspspName: string;
  aspspCountry: string;
  endsAt: string;
  sessionIdEnc?: string | null;
  superseded: boolean;
  weCanEndIt: boolean;
}

export interface SessionIdEnc {
  sessionId?: string | null;
}

// ---- Decrypted envelope payloads (the camelCase contract with the backend) -----------------------

export interface AccountEnc {
  ownerName?: string | null;
  iban?: string | null;
  bban?: string | null;
  accountName?: string | null;
  product?: string | null;
}

export interface DisplayNameEnc {
  displayName?: string | null;
}

export interface UidEnc {
  uid?: string | null;
}

export interface BalanceEnc {
  amount?: string | null;
  name?: string | null;
}

export interface TransactionEnc {
  amount?: string | null;
  creditorName?: string | null;
  creditorIban?: string | null;
  creditorBban?: string | null;
  creditorAgentBic?: string | null;
  debtorName?: string | null;
  debtorIban?: string | null;
  debtorBban?: string | null;
  debtorAgentBic?: string | null;
  remittanceInformation?: string | null;
  note?: string | null;
  referenceNumber?: string | null;
  exchangeRate?: string | null;
  merchantCategoryCode?: string | null;
  balanceAfter?: string | null;
  balanceAfterCurrency?: string | null;
  rawJson?: string | null;
}
