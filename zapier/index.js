'use strict';

const authentication = require('./authentication');
const newTransactionTrigger = require('./triggers/new_transaction');
const listAccountsSearch = require('./searches/list_accounts');
const listTransactionsSearch = require('./searches/list_transactions');
const syncAccountCreate = require('./creates/sync_account');

const pkg = require('./package.json');

module.exports = {
  version: pkg.version,
  platformVersion: require('zapier-platform-core').version,

  appCard: {
    title: 'open-banking.io',
    description: 'Connect your bank accounts and pull transactions into Zapier workflows. Zero-knowledge decryption — your data is encrypted end-to-end and decrypted locally.',
  },

  authentication,

  triggers: [newTransactionTrigger],

  searches: [listAccountsSearch, listTransactionsSearch],

  creates: [syncAccountCreate],
};
