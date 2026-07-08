'use strict';

const {
  resolveBundle,
  apiRequest,
  importBundleKey,
  mapTransaction,
  collectTransactionWires,
  transactionsPath,
} = require('../lib/client');

module.exports = {
  key: 'list_transactions',
  noun: 'Transaction',
  display: {
    label: 'Find Transactions',
    description: 'Lists transactions for a specific account with decrypted details.',
  },
  operation: {
    inputFields: [
      {
        key: 'accountId',
        type: 'string',
        label: 'Account ID',
        required: true,
        helpText: 'The account whose transactions to fetch.',
        dynamic: 'list_accounts.id.aspspName',
      },
      {
        key: 'from',
        type: 'string',
        label: 'From Date',
        required: false,
        helpText: 'Only transactions booked on or after this date (YYYY-MM-DD).',
        placeholder: 'YYYY-MM-DD',
      },
      {
        key: 'to',
        type: 'string',
        label: 'To Date',
        required: false,
        helpText: 'Only transactions booked on or before this date (YYYY-MM-DD).',
        placeholder: 'YYYY-MM-DD',
      },
      {
        key: 'limit',
        type: 'integer',
        label: 'Limit',
        required: false,
        default: '50',
        helpText: 'Maximum number of transactions to return.',
      },
    ],
    perform: async (z, bundle) => {
      const bundleResolved = resolveBundle(bundle.authData);
      const key = await importBundleKey(bundleResolved);
      const accountId = bundle.inputData.accountId;
      // ?? (not ||) so an explicit 0 is honoured and returns no rows.
      const limit = Number(bundle.inputData.limit ?? 50);

      const qs = {};
      if (bundle.inputData.from) qs.from = bundle.inputData.from;
      if (bundle.inputData.to) qs.to = bundle.inputData.to;

      const path = transactionsPath(accountId);
      const wires = await collectTransactionWires(
        (offset, lim) =>
          apiRequest(z, bundleResolved, 'GET', path, { ...qs, offset, limit: lim }),
        limit,
      );

      const transactions = await Promise.all(
        wires.map((w) => mapTransaction(key, w)),
      );
      return transactions.map((t) => ({ accountId, ...t }));
    },
    sample: {
      id: 'txn_001',
      accountId: 'acc_001',
      currency: 'DKK',
      amount: '42.50',
      creditDebitIndicator: 'DBIT',
      bookingDate: '2026-07-01',
      debtorName: 'REMA 1000',
    },
  },
};
