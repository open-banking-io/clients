'use strict';

const { expect } = require('chai');
const { importBundleKey, collectTransactionWires, mapTransaction, transactionsPath } = require('../lib/client');

// Drives the trigger polling logic with a faked Zapier z context.
// No envelopes — wires with a null `enc` decrypt to nulls, which is enough
// to exercise dispatch, the cross-poll cursor, and dedup.

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
 * Builds a Zapier z + bundle stand-in that serves a fixed set of transaction wires.
 * Mimics what the trigger's perform() expects.
 */
function makeZContext(state, wires) {
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
      const filtered = from ? wires.filter((w) => (w.bookingDate ?? '') >= from) : wires;
      const items = filtered.slice(offset, offset + limit);
      return { data: { items, total: filtered.length } };
    }
    throw new Error(`unexpected request: ${url}`);
  };

  const bundleResolved = {
    apiBaseUrl: 'http://localhost:8081',
    apiKey: 'k',
    privateKey: JSON.parse(BUNDLE).encryptionKey.privateKey,
  };

  const bundle = {
    inputData: { lookbackDays: 7 },
    authData: bundleResolved,
    meta: state.meta || {},
  };

  const z = {
    request: requestFn,
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

  return { z, bundle };
}

// Inline the trigger perform logic (extracted from triggers/new_transaction.js)
// so we can test it without the full Zapier platform.
async function performTrigger(z, bundle) {
  const { resolveBundle: _rb } = require('../lib/client');
  const bundleResolved = {
    apiBaseUrl: bundle.authData.apiBaseUrl,
    apiKey: bundle.authData.apiKey,
    privateKey: bundle.authData.privateKey,
  };

  const key = await importBundleKey(bundleResolved);
  const lookbackDays = bundle.inputData.lookbackDays || 7;

  let state = { lastBookingDate: null, seenIds: [] };
  const cursorRaw = bundle.meta?.cursor;
  if (cursorRaw) {
    try { state = JSON.parse(cursorRaw); } catch { /* start fresh */ }
  }

  const from = state.lastBookingDate ?? isoDaysAgo(lookbackDays);

  const accounts = await (async () => {
    const res = await z.request({
      method: 'GET',
      url: bundleResolved.apiBaseUrl + '/api/accounts',
      headers: { 'X-Api-Key': bundleResolved.apiKey },
      params: {},
      json: true,
    });
    return res.data;
  })();

  const seen = new Set(state.seenIds ?? []);
  const fresh = [];
  const emitted = [];
  let maxBookingDate = from;

  for (const account of accounts) {
    const path = transactionsPath(account.id);
    const wires = await collectTransactionWires(
      (offset, limit) =>
        z.request({
          method: 'GET',
          url: bundleResolved.apiBaseUrl + path,
          headers: { 'X-Api-Key': bundleResolved.apiKey },
          params: { from, offset, limit },
          json: true,
        }).then((res) => res.data),
      Number.POSITIVE_INFINITY,
    );

    for (const wire of wires) {
      if (seen.has(wire.id)) continue;
      seen.add(wire.id);
      const bookingDate = wire.bookingDate ?? null;
      emitted.push({ id: wire.id, bookingDate });
      const decrypted = await mapTransaction(key, wire);
      fresh.push({ id: decrypted.id, accountId: account.id, ...decrypted });
      if (bookingDate && bookingDate > maxBookingDate) {
        maxBookingDate = bookingDate;
      }
    }
  }

  const boundaryIds = emitted
    .filter((e) => e.bookingDate == null || e.bookingDate === maxBookingDate)
    .map((e) => e.id);

  state = {
    lastBookingDate: maxBookingDate,
    seenIds:
      maxBookingDate === from
        ? [...new Set([...(state.seenIds ?? []), ...boundaryIds])]
        : boundaryIds,
  };

  if (bundle.meta) {
    bundle.meta.cursor = JSON.stringify(state);
  }

  return fresh;
}

function isoDaysAgo(days) {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - days);
  return d.toISOString().slice(0, 10);
}

describe('Trigger polling cursor + dedup', () => {
  it('emits new transactions and persists the cursor', async () => {
    const meta = { cursor: JSON.stringify({ lastBookingDate: '2026-06-07', seenIds: [] }) };
    const { z, bundle } = makeZContext({ meta }, [
      wire('t1', '2026-06-07'),
      wire('t2', '2026-06-08'),
      wire('t3', '2026-06-08'),
    ]);

    const out = await performTrigger(z, bundle);

    expect(out).to.have.length(3);
    const cursor = JSON.parse(bundle.meta.cursor);
    expect(cursor.lastBookingDate).to.equal('2026-06-08');
    // Cursor advanced past 06-07, so t1 is dropped; only the new boundary date's IDs are kept.
    expect(cursor.seenIds).to.deep.equal(['t2', 't3']);
  });

  it('does not re-emit boundary-date rows on the next poll', async () => {
    const meta = { cursor: JSON.stringify({ lastBookingDate: '2026-06-08', seenIds: ['t2', 't3'] }) };
    const { z, bundle } = makeZContext({ meta }, [
      wire('t2', '2026-06-08'),
      wire('t3', '2026-06-08'),
      wire('t4', '2026-06-09'),
    ]);

    const out = await performTrigger(z, bundle);

    expect(out).to.have.length(1);
    expect(out[0].id).to.equal('t4');
    const cursor = JSON.parse(bundle.meta.cursor);
    expect(cursor.lastBookingDate).to.equal('2026-06-09');
    expect(cursor.seenIds).to.deep.equal(['t4']);
  });

  it('accumulates same-day IDs when the cursor does not advance', async () => {
    const meta = { cursor: JSON.stringify({ lastBookingDate: '2026-06-08', seenIds: ['t2', 't3'] }) };
    const { z, bundle } = makeZContext({ meta }, [
      wire('t2', '2026-06-08'),
      wire('t3', '2026-06-08'),
      wire('t9', '2026-06-08'),
    ]);

    const out = await performTrigger(z, bundle);

    expect(out).to.have.length(1);
    expect(out[0].id).to.equal('t9');
    const cursor = JSON.parse(bundle.meta.cursor);
    expect(cursor.lastBookingDate).to.equal('2026-06-08');
    // Date held steady, so prior boundary IDs are kept and the new one is added (union).
    expect(cursor.seenIds.sort()).to.deep.equal(['t2', 't3', 't9']);
  });
});
