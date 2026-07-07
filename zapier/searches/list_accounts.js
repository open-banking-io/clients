'use strict';

const { apiRequest, importBundleKey, mapAccount } = require('../lib/client');

module.exports = {
  key: 'list_accounts',
  noun: 'Account',
  display: {
    label: 'Find Account',
    description: 'Lists all bank accounts with decrypted details and balances.',
  },
  operation: {
    perform: async (z, bundle) => {
      const bundleResolved = {
        apiBaseUrl: bundle.authData.apiBaseUrl,
        apiKey: bundle.authData.apiKey,
        privateKey: bundle.authData.privateKey,
      };
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
