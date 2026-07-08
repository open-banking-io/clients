'use strict';

const { resolveBundle, apiRequest, importBundleKey, mapAccount } = require('../lib/client');

// Hidden trigger that powers the dynamic "Account ID" dropdowns.
// Input-field `dynamic` references must point at a *trigger* key — the
// list_accounts *search* cannot serve that role.

module.exports = {
  key: 'list_accounts',
  noun: 'Account',
  display: {
    label: 'List Accounts',
    description: 'Lists bank accounts (powers the account dropdowns).',
    hidden: true,
  },
  operation: {
    perform: async (z, bundle) => {
      const bundleResolved = resolveBundle(bundle.authData);
      const key = await importBundleKey(bundleResolved);
      const wires = await apiRequest(z, bundleResolved, 'GET', '/api/accounts');
      return Promise.all(wires.map((w) => mapAccount(key, w)));
    },
    sample: {
      id: 'acc_001',
      aspspName: 'Danske Bank',
      aspspCountry: 'DK',
      currency: 'DKK',
      iban: 'DK1234567890123456',
      ownerName: 'John Doe',
      displayName: 'Checking',
      balances: [
        { type: 'expected', amount: '1234.56', currency: 'DKK' },
      ],
    },
  },
};
