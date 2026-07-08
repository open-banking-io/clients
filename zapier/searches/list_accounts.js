'use strict';

const { listAccounts } = require('../lib/client');

module.exports = {
  key: 'list_accounts',
  noun: 'Account',
  display: {
    label: 'Find Account',
    description: 'Lists all bank accounts with decrypted details and balances.',
  },
  operation: {
    perform: (z, bundle) => listAccounts(z, bundle.authData),
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
