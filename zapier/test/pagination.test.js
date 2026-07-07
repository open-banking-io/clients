'use strict';

const { expect } = require('chai');
const { collectTransactionWires } = require('../lib/client');

/** A fake paged endpoint over `total` synthetic transactions. */
function pagedSource(total) {
  const calls = [];
  const fn = async (offset, limit) => {
    calls.push({ offset, limit });
    const items = [];
    for (let i = offset; i < Math.min(offset + limit, total); i++) {
      items.push({ id: `t${i}`, currency: 'DKK', creditDebitIndicator: 'DBIT' });
    }
    return { items, total };
  };
  fn.calls = calls;
  return fn;
}

describe('collectTransactionWires', () => {
  it('collects every page when returning all', async () => {
    const fetchPage = pagedSource(250);
    const wires = await collectTransactionWires(fetchPage, Number.POSITIVE_INFINITY, 100);
    expect(wires).to.have.length(250);
    expect(fetchPage.calls).to.have.length(3); // 100 + 100 + 50
  });

  it('stops at the user limit', async () => {
    const fetchPage = pagedSource(250);
    const wires = await collectTransactionWires(fetchPage, 120, 100);
    expect(wires).to.have.length(120);
    // Second page is requested with the remaining 20, never overshooting.
    const lastCall = fetchPage.calls[fetchPage.calls.length - 1];
    expect(lastCall).to.deep.equal({ offset: 100, limit: 20 });
  });

  it('stops on an empty page', async () => {
    const fetchPage = async () => ({ items: [], total: 999 });
    const wires = await collectTransactionWires(fetchPage, Number.POSITIVE_INFINITY, 100);
    expect(wires).to.have.length(0);
  });

  it('returns exactly total when total is below a single page', async () => {
    const fetchPage = pagedSource(7);
    const wires = await collectTransactionWires(fetchPage, Number.POSITIVE_INFINITY, 100);
    expect(wires).to.have.length(7);
    expect(fetchPage.calls).to.have.length(1);
  });
});
