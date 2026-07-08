'use strict';

const { decryptTo, importPrivateKey } = require('./envelope');

/** The fields we need out of the pasted credentials bundle. */
/**
 * @typedef {Object} Bundle
 * @property {string} apiBaseUrl
 * @property {string} apiKey
 * @property {string} privateKey
 */

/**
 * Trims trailing '/' characters without regex backtracking
 * (a `/\/+$/` replace is flagged by CodeQL as polynomial ReDoS).
 * @param {string} value
 * @returns {string}
 */
function stripTrailingSlashes(value) {
  let end = value.length;
  while (end > 0 && value.charCodeAt(end - 1) === 47 /* '/' */) end -= 1;
  return value.slice(0, end);
}

/**
 * Parses and validates the credentials-bundle JSON the user pasted into auth.
 * @param {string} raw — the bundle JSON string
 * @returns {Bundle}
 */
function parseBundle(raw) {
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch {
    throw new Error('Credentials bundle is not valid JSON');
  }

  const apiBaseUrl = stripTrailingSlashes(String(parsed.apiBaseUrl ?? ''));
  const apiKey = String(parsed.apiKey ?? '');
  const encryptionKey = parsed.encryptionKey ?? {};
  const privateKey = String(encryptionKey.privateKey ?? '');

  if (!apiBaseUrl) throw new Error('Credentials bundle is missing "apiBaseUrl"');
  if (!apiKey) throw new Error('Credentials bundle is missing "apiKey"');
  if (!privateKey) throw new Error('Credentials bundle is missing "encryptionKey.privateKey"');

  return { apiBaseUrl, apiKey, privateKey };
}

/**
 * Resolves the bundle from authData, applying an optional base URL override.
 * @param {Object} authData — Zapier authData containing bundle + optional baseUrlOverride
 * @returns {Bundle}
 */
function resolveBundle(authData) {
  const bundle = parseBundle(authData.bundle);
  const override = stripTrailingSlashes(String(authData.baseUrlOverride ?? '').trim());
  if (override) bundle.apiBaseUrl = override;
  return bundle;
}

/**
 * Issues an authenticated request through Zapier's z.request().
 * The X-Api-Key header is injected from the resolved bundle.
 *
 * @param {Object} z — Zapier's z object
 * @param {Bundle} bundle — resolved credentials bundle
 * @param {string} method — HTTP method (GET, POST, etc.)
 * @param {string} path — API path (e.g. '/api/accounts')
 * @param {Object} [qs] — query string params
 * @param {Object} [body] — request body
 * @returns {Promise<any>}
 */
async function apiRequest(z, bundle, method, path, qs, body) {
  const options = {
    method,
    url: bundle.apiBaseUrl + path,
    headers: {
      'X-Api-Key': bundle.apiKey,
    },
    params: qs
      ? Object.fromEntries(Object.entries(qs).map(([k, v]) => [k, String(v)]))
      : undefined,
    body,
    json: true,
  };
  const res = await z.request(options);
  return res.data;
}

/** Imports the bundle's private key once for reuse. */
function importBundleKey(bundle) {
  return importPrivateKey(bundle.privateKey);
}

/**
 * Decrypts and maps an AccountWire to a plain Account object.
 * @param {CryptoKey} key
 * @param {AccountWire} a
 */
async function mapAccount(key, a) {
  const acc = await decryptTo(key, a.enc);
  const name = await decryptTo(key, a.displayNameEnc);
  const balances = await Promise.all(
    (a.balances ?? []).map(async (b) => {
      const dec = await decryptTo(key, b.enc);
      return {
        type: b.type,
        name: dec?.name ?? null,
        amount: dec?.amount ?? '0',
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

/**
 * Decrypts and maps a TransactionWire to a plain Transaction object.
 * @param {CryptoKey} key
 * @param {TransactionWire} t
 */
async function mapTransaction(key, t) {
  const d = await decryptTo(key, t.enc);
  return {
    id: t.id,
    currency: t.currency,
    creditDebitIndicator: t.creditDebitIndicator,
    status: t.status ?? null,
    bookingDate: t.bookingDate ?? null,
    valueDate: t.valueDate ?? null,
    transactionDate: t.transactionDate ?? null,
    bankTransactionCode: t.bankTransactionCode ?? null,
    amount: d?.amount ?? '0',
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

/**
 * Maps a ConnectionWire to a plain Connection object (no decryption needed).
 * @param {ConnectionWire} c
 */
function mapConnection(c) {
  return {
    sessionId: c.sessionId,
    aspspName: c.aspspName,
    aspspCountry: c.aspspCountry,
    validUntil: c.validUntil,
    status: c.status,
    accountCount: c.accountCount,
    lastSyncedAt: c.lastSyncedAt ?? null,
    psuType: c.psuType ?? null,
  };
}

/**
 * Decrypts an account's session uid, or returns null if there's no active session.
 * @param {CryptoKey} key
 * @param {AccountWire} a
 * @returns {Promise<string | null>}
 */
async function decryptUid(key, a) {
  const dec = await decryptTo(key, a.uidEnc);
  return dec?.uid ?? null;
}

/**
 * Collects transaction wires by looping offset/limit until total is reached,
 * a page comes back empty, or userLimit rows are gathered.
 * Pure (page fetch is injected) so it can be unit-tested without a Zapier context.
 *
 * @param {(offset: number, limit: number) => Promise<TransactionPageWire>} fetchPage
 * @param {number} userLimit
 * @param {number} [pageSize=100]
 * @returns {Promise<TransactionWire[]>}
 */
async function collectTransactionWires(fetchPage, userLimit, pageSize = 100) {
  const collected = [];
  let offset = 0;
  let total = Number.POSITIVE_INFINITY;

  while (collected.length < Math.min(userLimit, total)) {
    const remaining = userLimit - collected.length;
    const limit = Math.min(pageSize, remaining);
    const page = await fetchPage(offset, limit);
    total = page.total;
    const items = page.items ?? [];
    if (items.length === 0) break;
    collected.push(...items);
    offset += items.length;
    if (collected.length >= total) break;
  }

  return collected.length > userLimit ? collected.slice(0, userLimit) : collected;
}

/** @param {string} accountId */
function transactionsPath(accountId) {
  return `/api/accounts/${encodeURIComponent(accountId)}/transactions`;
}

module.exports = {
  parseBundle,
  resolveBundle,
  apiRequest,
  importBundleKey,
  mapAccount,
  mapTransaction,
  mapConnection,
  decryptUid,
  collectTransactionWires,
  transactionsPath,
};
