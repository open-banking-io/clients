'use strict';

// Self-contained copy of the open-banking.io domain + wire types.
// Duplicated here (rather than imported from @open-banking-io/client) so the
// Zapier app has zero runtime dependencies.
//
// Monetary amounts are decimal STRINGS throughout — never parse them to floats.

/** @typedef {{ type: string, currency: string, referenceDate: ?string, enc: ?string }} BalanceWire */
/** @typedef {{ id: string, aspspName: string, aspspCountry: string, currency: string, accountType: ?string, bic: ?string, needsReconnect: boolean, balances: BalanceWire[], enc: ?string, displayNameEnc: ?string, uidEnc: ?string }} AccountWire */
/** @typedef {{ id: string, currency: string, creditDebitIndicator: string, status: ?string, bookingDate: ?string, valueDate: ?string, transactionDate: ?string, bankTransactionCode: ?string, enc: ?string }} TransactionWire */
/** @typedef {{ items: TransactionWire[], total: number }} TransactionPageWire */
/** @typedef {{ sessionId: string, aspspName: string, aspspCountry: string, validUntil: string, status: string, accountCount: number, lastSyncedAt: ?string, psuType: ?string }} ConnectionWire */
/** @typedef {{ newTransactions: number, totalFetched: number }} SyncResultWire */
/** @typedef {{ accounts: number, newTransactions: number }} SyncAllResultWire */

module.exports = {};
