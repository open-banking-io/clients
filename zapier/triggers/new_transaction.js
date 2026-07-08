'use strict';

const {
  resolveBundle,
  apiRequest,
  importBundleKey,
  mapTransaction,
  collectTransactionWires,
  transactionsPath,
} = require('../lib/client');

// Polling trigger: fires when new bank transactions arrive across all accounts.
//
// Cursor + dedup strategy (same algorithm as the n8n trigger):
//   - On first poll, fetch from `lookbackDays` days ago.
//   - On subsequent polls, fetch from the last booking date seen.
//   - Track IDs seen on the boundary date to dedupe same-day re-fetches.
//   - State is persisted between polls as JSON via z.cursor (requires canPaginate).

const TRIGGER_KEY = 'new_transaction';

const trigger = {
  key: TRIGGER_KEY,
  noun: 'Transaction',
  display: {
    label: 'New Transaction',
    description: 'Triggers when a new bank transaction arrives on open-banking.io.',
  },
  operation: {
    inputFields: [
      {
        key: 'lookbackDays',
        type: 'integer',
        label: 'Initial Lookback (Days)',
        helpText:
          'On the first poll, how many days back to fetch transactions from. ' +
          'Later polls only fetch transactions newer than the last one seen.',
        default: '7',
      },
    ],
    type: 'polling',
    canPaginate: true,
    perform: async (z, bundle) => {
      const bundleResolved = resolveBundle(bundle.authData);

      const key = await importBundleKey(bundleResolved);
      const lookbackDays = Number(bundle.inputData.lookbackDays ?? 7);

      // Parse cursor state persisted by the previous poll (JSON string).
      let state = { lastBookingDate: null, seenIds: [] };
      const cursorRaw = await z.cursor.get();
      if (cursorRaw) {
        try {
          state = JSON.parse(cursorRaw);
        } catch {
          // Corrupt cursor — start fresh with lookback.
        }
      }

      const from = state.lastBookingDate ?? isoDaysAgo(lookbackDays);

      // Fetch all accounts, then transactions for each from the cursor date.
      const accounts = await apiRequest(z, bundleResolved, 'GET', '/api/accounts');
      const seen = new Set(state.seenIds ?? []);
      const fresh = [];
      const emitted = [];
      let maxBookingDate = from;

      for (const account of accounts) {
        const path = transactionsPath(account.id);
        const wires = await collectTransactionWires(
          (offset, limit) =>
            apiRequest(z, bundleResolved, 'GET', path, { from, offset, limit }),
          Number.POSITIVE_INFINITY,
        );

        for (const wire of wires) {
          if (seen.has(wire.id)) continue;
          seen.add(wire.id);
          const bookingDate = wire.bookingDate ?? null;
          emitted.push({ id: wire.id, bookingDate });
          const decrypted = await mapTransaction(key, wire);
          fresh.push({
            id: decrypted.id,
            accountId: account.id,
            ...decrypted,
          });
          if (bookingDate && bookingDate > maxBookingDate) {
            maxBookingDate = bookingDate;
          }
        }
      }

      // Advance the cursor: store the new max date + boundary IDs.
      const boundaryIds = emitted
        .filter((e) => e.bookingDate == null || e.bookingDate === maxBookingDate)
        .map((e) => e.id);

      const newState = {
        lastBookingDate: maxBookingDate,
        seenIds:
          maxBookingDate === from
            ? [...new Set([...(state.seenIds ?? []), ...boundaryIds])]
            : boundaryIds,
      };

      // Persist the cursor for the next poll.
      await z.cursor.set(JSON.stringify(newState));

      return fresh;
    },
    sample: {
      id: 'txn_001',
      accountId: 'acc_001',
      currency: 'EUR',
      amount: '42.50',
      creditDebitIndicator: 'CRDT',
      bookingDate: '2026-07-01',
      debtorName: 'Acme Corp',
      remittanceInformation: 'Invoice #12345',
    },
    outputFields: [
      { key: 'id', label: 'Transaction ID' },
      { key: 'accountId', label: 'Account ID' },
      { key: 'currency', label: 'Currency' },
      { key: 'amount', label: 'Amount (decimal string)' },
      { key: 'creditDebitIndicator', label: 'Credit/Debit' },
      { key: 'bookingDate', label: 'Booking Date' },
      { key: 'debtorName', label: 'Debtor Name' },
      { key: 'creditorName', label: 'Creditor Name' },
      { key: 'remittanceInformation', label: 'Remittance Information' },
    ],
  },
};

/** Returns the YYYY-MM-DD date `days` days before today (UTC). */
function isoDaysAgo(days) {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - days);
  return d.toISOString().slice(0, 10);
}

module.exports = trigger;
