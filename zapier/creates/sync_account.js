'use strict';

const { resolveBundle, apiRequest, importBundleKey, decryptUid } = require('../lib/client');

module.exports = {
  key: 'sync_account',
  noun: 'Sync',
  display: {
    label: 'Sync Account',
    description: 'Pull fresh transactions for one account from the bank.',
  },
  operation: {
    inputFields: [
      {
        key: 'accountId',
        type: 'string',
        label: 'Account ID',
        required: true,
        helpText: 'The account to sync.',
        dynamic: 'list_accounts.id.aspspName',
      },
    ],
    perform: async (z, bundle) => {
      const bundleResolved = resolveBundle(bundle.authData);
      const key = await importBundleKey(bundleResolved);
      const accountId = bundle.inputData.accountId;

      // Fetch accounts, find the target, decrypt its session uid.
      const wires = await apiRequest(z, bundleResolved, 'GET', '/api/accounts');
      const account = wires.find((a) => a.id === accountId);
      if (!account) {
        throw new z.errors.Error(
          `Account ${accountId} not found`,
          'NOT_FOUND',
          404,
        );
      }

      const uid = await decryptUid(key, account);
      if (uid == null) {
        throw new z.errors.Error(
          'Account has no active session (reconnect required) — cannot sync',
          'SESSION_EXPIRED',
          400,
        );
      }

      const result = await apiRequest(
        z,
        bundleResolved,
        'POST',
        `/api/accounts/${encodeURIComponent(accountId)}/sync`,
        undefined,
        { uid },
      );

      return {
        accountId,
        newTransactions: result.newTransactions,
        totalFetched: result.totalFetched,
      };
    },
    sample: {
      accountId: 'acc_001',
      newTransactions: 5,
      totalFetched: 42,
    },
  },
};
