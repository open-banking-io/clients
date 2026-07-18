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
}

export interface SyncResult {
  newTransactions: number;
  totalFetched: number;
}

export interface SyncAllResult {
  accounts: number;
  newTransactions: number;
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
}

export interface SyncResultWire {
  newTransactions: number;
  totalFetched: number;
}

export interface SyncAllResultWire {
  accounts: number;
  newTransactions: number;
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
