'use strict';

const { listAccounts } = require('../lib/client');

module.exports = {
  key: 'list_accounts',
  noun: 'Account',
  display: {
    label: 'Find Account',
    description: 'Finds bank accounts by IBAN, name or bank — or lists all accounts.',
  },
  operation: {
    // Zapier integration check D009: a search needs at least one input field.
    inputFields: [
      {
        key: 'query',
        type: 'string',
        label: 'Search Term',
        required: false,
        helpText:
          'Optional. Case-insensitive match against IBAN, BBAN, account name, ' +
          'display name, owner name and bank name. Leave empty to return all accounts.',
      },
    ],
    perform: async (z, bundle) => {
      const accounts = await listAccounts(z, bundle.authData);
      const query = String(bundle.inputData.query ?? '').trim().toLowerCase();
      if (!query) return accounts;
      return accounts.filter((a) =>
        [a.iban, a.bban, a.accountName, a.displayName, a.ownerName, a.aspspName]
          .filter(Boolean)
          .some((v) => String(v).toLowerCase().includes(query)),
      );
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
