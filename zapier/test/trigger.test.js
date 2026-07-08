'use strict';

const { expect } = require('chai');

const trigger = require('../triggers/new_transaction');

// Drives the REAL trigger perform() with a faked Zapier z context (request +
// cursor store). No envelopes — wires with a null `enc` decrypt to nulls, which
// is enough to exercise dispatch, the cross-poll cursor, and dedup.

const BUNDLE = JSON.stringify({
  apiBaseUrl: 'http://localhost:8081',
  apiKey: 'k',
  encryptionKey: {
    // Valid P-256 PKCS#8 key from the shared fixtures (only needs to import, not decrypt).
    privateKey:
      'MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgxz8NJk/nEf3HI8gndLwczyCHauSP6rAZlGZN2Ih9FeKhRANCAARutFZRcZlU2oa5FD3PJhCnBXI0qPqQe5h3zJVBwcfmx0j3S39p9cus3+no2rxfRKJC+lLk5NDTC+xpy0INTyMH',
  },
});

function wire(id, bookingDate) {
  return { id, currency: 'DKK', creditDebitIndicator: 'DBIT', bookingDate, enc: null };
}

/**
 * Builds a Zapier z + bundle stand-in that serves a fixed set of transaction
 * wires and persists the cursor in-memory, mimicking z.cursor.
 */
function makeZContext(initialCursor, wires) {
  const cursorStore = { value: initialCursor ?? null };

  const requestFn = async (options) => {
    const url = options.url.replace('http://localhost:8081', '');
    const params = options.params || {};

    if (url === '/api/accounts') {
      return { data: [{ id: 'acc-1', balances: [] }] };
    }
    if (url.includes('/transactions')) {
      const from = params.from;
      const offset = Number(params.offset || 0);
      const limit = Number(params.limit || 100);
      // Include null-bookingDate (pending) rows regardless of `from`, like the API does.
      const filtered = from
        ? wires.filter((w) => w.bookingDate == null || w.bookingDate >= from)
        : wires;
      const items = filtered.slice(offset, offset + limit);
      return { data: { items, total: filtered.length } };
    }
    throw new Error(`unexpected request: ${url}`);
  };

  const bundle = {
    inputData: { lookbackDays: 7 },
    authData: { bundle: BUNDLE },
    meta: {},
  };

  const z = {
    request: requestFn,
    cursor: {
      get: async () => cursorStore.value,
      set: async (value) => {
        cursorStore.value = value;
      },
    },
    errors: {
      Error: class ZError extends Error {
        constructor(message, code, status) {
          super(message);
          this.code = code;
          this.status = status;
        }
      },
    },
  };

  return { z, bundle, cursorStore };
}

const perform = trigger.operation.perform;

describe('Trigger polling cursor + dedup (real module)', () => {
  it('declares itself a polling trigger with cursor support', () => {
    expect(trigger.operation.type).to.equal('polling');
    expect(trigger.operation.canPaginate).to.equal(true);
  });

  it('emits new transactions and persists the cursor', async () => {
    const initial = JSON.stringify({ lastBookingDate: '2026-06-07', seenIds: [] });
    const { z, bundle, cursorStore } = makeZContext(initial, [
      wire('t1', '2026-06-07'),
      wire('t2', '2026-06-08'),
      wire('t3', '2026-06-08'),
    ]);

    const out = await perform(z, bundle);

    expect(out).to.have.length(3);
    const cursor = JSON.parse(cursorStore.value);
    expect(cursor.lastBookingDate).to.equal('2026-06-08');
    // Cursor advanced past 06-07, so t1 is dropped; only the new boundary date's IDs are kept.
    expect(cursor.seenIds).to.deep.equal(['t2', 't3']);
  });

  it('does not re-emit boundary-date rows on the next poll', async () => {
    const initial = JSON.stringify({ lastBookingDate: '2026-06-08', seenIds: ['t2', 't3'] });
    const { z, bundle, cursorStore } = makeZContext(initial, [
      wire('t2', '2026-06-08'),
      wire('t3', '2026-06-08'),
      wire('t4', '2026-06-09'),
    ]);

    const out = await perform(z, bundle);

    expect(out).to.have.length(1);
    expect(out[0].id).to.equal('t4');
    const cursor = JSON.parse(cursorStore.value);
    expect(cursor.lastBookingDate).to.equal('2026-06-09');
    expect(cursor.seenIds).to.deep.equal(['t4']);
  });

  it('accumulates same-day IDs when the cursor does not advance', async () => {
    const initial = JSON.stringify({ lastBookingDate: '2026-06-08', seenIds: ['t2', 't3'] });
    const { z, bundle, cursorStore } = makeZContext(initial, [
      wire('t2', '2026-06-08'),
      wire('t3', '2026-06-08'),
      wire('t9', '2026-06-08'),
    ]);

    const out = await perform(z, bundle);

    expect(out).to.have.length(1);
    expect(out[0].id).to.equal('t9');
    const cursor = JSON.parse(cursorStore.value);
    expect(cursor.lastBookingDate).to.equal('2026-06-08');
    // Date held steady, so prior boundary IDs are kept and the new one is added (union).
    expect(cursor.seenIds.sort()).to.deep.equal(['t2', 't3', 't9']);
  });

  it('emits pending (null-bookingDate) transactions and tracks them separately', async () => {
    const initial = JSON.stringify({ lastBookingDate: '2026-06-08', seenIds: [] });
    const { z, bundle, cursorStore } = makeZContext(initial, [
      wire('t5', '2026-06-08'),
      wire('p1', null),
    ]);

    const out = await perform(z, bundle);

    expect(out.map((t) => t.id).sort()).to.deep.equal(['p1', 't5']);
    const cursor = JSON.parse(cursorStore.value);
    // A null bookingDate must not advance the cursor date…
    expect(cursor.lastBookingDate).to.equal('2026-06-08');
    // …and the pending row is remembered separately from the boundary-date IDs.
    expect(cursor.seenIds).to.deep.equal(['t5']);
    expect(cursor.pendingIds).to.deep.equal(['p1']);
  });

  it('does not re-emit a pending row after the boundary date advances', async () => {
    // Poll 1 saw pending p1; poll 2 advances the date — p1 must stay remembered.
    const initial = JSON.stringify({
      lastBookingDate: '2026-06-08',
      seenIds: ['t5'],
      pendingIds: ['p1'],
    });
    const { z, bundle, cursorStore } = makeZContext(initial, [
      wire('t5', '2026-06-08'),
      wire('p1', null),
      wire('t6', '2026-06-09'),
    ]);

    const out = await perform(z, bundle);

    expect(out.map((t) => t.id)).to.deep.equal(['t6']);
    const cursor = JSON.parse(cursorStore.value);
    expect(cursor.lastBookingDate).to.equal('2026-06-09');
    expect(cursor.seenIds).to.deep.equal(['t6']);
    expect(cursor.pendingIds).to.deep.equal(['p1']);
  });

  it('does not re-emit a pending row once it finalizes with a real bookingDate', async () => {
    // Intended behavior: a transaction fires the Zap exactly once — at first
    // sight, while pending. When the bank later books it (same id, real date),
    // it must NOT fire again. Zapier's id-deduper would suppress a re-emit
    // anyway; the cursor logic matches that contract.
    const initial = JSON.stringify({
      lastBookingDate: '2026-06-08',
      seenIds: [],
      pendingIds: ['p1'],
    });
    const { z, bundle, cursorStore } = makeZContext(initial, [
      wire('p1', '2026-06-09'), // finalized: was pending, now booked
      wire('t7', '2026-06-09'),
    ]);

    const out = await perform(z, bundle);

    expect(out.map((t) => t.id)).to.deep.equal(['t7']);
    const cursor = JSON.parse(cursorStore.value);
    expect(cursor.lastBookingDate).to.equal('2026-06-09');
    // p1 stays remembered as pending (capped list) — harmless.
    expect(cursor.pendingIds).to.deep.equal(['p1']);
  });

  it('returns results reverse-chronologically with pending rows last', async () => {
    const initial = JSON.stringify({ lastBookingDate: '2026-06-07', seenIds: [] });
    const { z, bundle } = makeZContext(initial, [
      wire('old', '2026-06-07'),
      wire('pending', null),
      wire('new', '2026-06-09'),
      wire('mid', '2026-06-08'),
    ]);

    const out = await perform(z, bundle);

    expect(out.map((t) => t.id)).to.deep.equal(['new', 'mid', 'old', 'pending']);
  });

  it('starts from scratch on a corrupt cursor', async () => {
    // The lookback window is relative to "now", so use today's date to stay inside it.
    const today = new Date().toISOString().slice(0, 10);
    const { z, bundle, cursorStore } = makeZContext('not json', [wire('t1', today)]);

    const out = await perform(z, bundle);

    expect(out).to.have.length(1);
    expect(JSON.parse(cursorStore.value).lastBookingDate).to.equal(today);
  });
});
