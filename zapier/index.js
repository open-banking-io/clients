'use strict';

const authentication = require('./authentication');
const newTransactionTrigger = require('./triggers/new_transaction');
const listAccountsTrigger = require('./triggers/list_accounts');
const listAccountsSearch = require('./searches/list_accounts');
const listTransactionsSearch = require('./searches/list_transactions');
const syncAccountCreate = require('./creates/sync_account');

const pkg = require('./package.json');

// AppSchema requires triggers/searches/creates to be objects keyed by each
// operation's `key` (arrays fail `zapier validate`).
module.exports = {
  version: pkg.version,
  platformVersion: require('zapier-platform-core').version,

  authentication,

  triggers: {
    [newTransactionTrigger.key]: newTransactionTrigger,
    [listAccountsTrigger.key]: listAccountsTrigger,
  },

  searches: {
    [listAccountsSearch.key]: listAccountsSearch,
    [listTransactionsSearch.key]: listTransactionsSearch,
  },

  creates: {
    [syncAccountCreate.key]: syncAccountCreate,
  },
};
