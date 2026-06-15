import { describe, expect, it, vi } from 'vitest';

import { collectTransactionWires } from '../nodes/OpenBankingIo/shared/client';
import type { TransactionPageWire, TransactionWire } from '../nodes/OpenBankingIo/shared/models';

function makeWire(id: string): TransactionWire {
	return { id, currency: 'DKK', creditDebitIndicator: 'DBIT' };
}

/** A fake paged endpoint over `total` synthetic transactions. */
function pagedSource(total: number) {
	return vi.fn(async (offset: number, limit: number): Promise<TransactionPageWire> => {
		const items: TransactionWire[] = [];
		for (let i = offset; i < Math.min(offset + limit, total); i++) {
			items.push(makeWire(`t${i}`));
		}
		return { items, total };
	});
}

describe('collectTransactionWires', () => {
	it('collects every page when returning all', async () => {
		const fetchPage = pagedSource(250);
		const wires = await collectTransactionWires(fetchPage, Number.POSITIVE_INFINITY, 100);
		expect(wires).toHaveLength(250);
		expect(fetchPage).toHaveBeenCalledTimes(3); // 100 + 100 + 50
	});

	it('stops at the user limit', async () => {
		const fetchPage = pagedSource(250);
		const wires = await collectTransactionWires(fetchPage, 120, 100);
		expect(wires).toHaveLength(120);
		// Second page is requested with the remaining 20, never overshooting.
		expect(fetchPage).toHaveBeenLastCalledWith(100, 20);
	});

	it('stops on an empty page', async () => {
		const fetchPage = vi.fn(async (): Promise<TransactionPageWire> => ({ items: [], total: 999 }));
		const wires = await collectTransactionWires(fetchPage, Number.POSITIVE_INFINITY, 100);
		expect(wires).toHaveLength(0);
		expect(fetchPage).toHaveBeenCalledTimes(1);
	});

	it('returns exactly total when total is below a single page', async () => {
		const fetchPage = pagedSource(7);
		const wires = await collectTransactionWires(fetchPage, Number.POSITIVE_INFINITY, 100);
		expect(wires).toHaveLength(7);
		expect(fetchPage).toHaveBeenCalledTimes(1);
	});
});
