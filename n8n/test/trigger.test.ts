import { describe, expect, it, vi } from 'vitest';

import { OpenBankingIoTrigger } from '../nodes/OpenBankingIo/OpenBankingIoTrigger.node';
import type { TransactionWire } from '../nodes/OpenBankingIo/shared/models';

// Drives the real trigger poll() with a faked n8n context. No envelopes — wires with a null `enc`
// decrypt to nulls, which is enough to exercise dispatch, the cross-poll cursor, and dedup.

const BUNDLE = JSON.stringify({
	apiBaseUrl: 'http://localhost:8081',
	apiKey: 'k',
	encryptionKey: {
		// Valid P-256 PKCS#8 key from the shared fixtures (only needs to import, not decrypt).
		privateKey:
			'MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgxz8NJk/nEf3HI8gndLwczyCHauSP6rAZlGZN2Ih9FeKhRANCAARutFZRcZlU2oa5FD3PJhCnBXI0qPqQe5h3zJVBwcfmx0j3S39p9cus3+no2rxfRKJC+lLk5NDTC+xpy0INTyMH',
	},
});

function wire(id: string, bookingDate: string): TransactionWire {
	return { id, currency: 'DKK', creditDebitIndicator: 'DBIT', bookingDate, enc: null };
}

/** Builds an IPollFunctions stand-in that serves a fixed set of transaction wires. */
function makeContext(mode: 'manual' | 'trigger', state: Record<string, unknown>, wires: TransactionWire[]) {
	const httpRequestWithAuthentication = vi.fn(async (_cred: string, options: { url: string; qs?: Record<string, string> }) => {
		const path = options.url.replace('http://localhost:8081', '');
		if (path === '/api/accounts') return [{ id: 'acc-1', balances: [] }];
		if (path.includes('/transactions')) {
			// Honor the `from` filter the trigger sends, like the real API.
			const from = options.qs?.from;
			const items = from ? wires.filter((w) => (w.bookingDate ?? '') >= from) : wires;
			return { items, total: items.length };
		}
		throw new Error(`unexpected request: ${path}`);
	});

	const ctx = {
		getCredentials: async () => ({ bundle: BUNDLE }),
		getNodeParameter: (_name: string, fallback?: unknown) => fallback,
		getWorkflowStaticData: () => state,
		getMode: () => mode,
		helpers: { httpRequestWithAuthentication },
	};
	return { ctx, httpRequestWithAuthentication };
}

describe('OpenBankingIoTrigger poll()', () => {
	it('emits new transactions and persists the cursor on a scheduled poll', async () => {
		// Seed the cursor so `from` is deterministic (independent of today's date).
		const state: Record<string, unknown> = { lastBookingDate: '2026-06-07' };
		const wires = [wire('t1', '2026-06-07'), wire('t2', '2026-06-08'), wire('t3', '2026-06-08')];
		const { ctx } = makeContext('trigger', state, wires);

		const out = await OpenBankingIoTrigger.prototype.poll.call(ctx as never);

		expect(out?.[0]).toHaveLength(3);
		expect(state.lastBookingDate).toBe('2026-06-08');
		// Cursor advanced past 06-07, so t1 is dropped; only the new boundary date's IDs are kept.
		expect(state.seenIds).toEqual(['t2', 't3']);
	});

	it('does not re-emit boundary-date rows on the next poll', async () => {
		const state: Record<string, unknown> = { lastBookingDate: '2026-06-08', seenIds: ['t2', 't3'] };
		// Same boundary-date rows come back (date filter is day-granular), plus one genuinely new row.
		const wires = [wire('t2', '2026-06-08'), wire('t3', '2026-06-08'), wire('t4', '2026-06-09')];
		const { ctx } = makeContext('trigger', state, wires);

		const out = await OpenBankingIoTrigger.prototype.poll.call(ctx as never);

		expect(out?.[0]).toHaveLength(1);
		expect(out?.[0][0].json.id).toBe('t4');
		expect(state.lastBookingDate).toBe('2026-06-09');
		// Cursor advanced, so the old boundary IDs are dropped and only the new date's ID is kept.
		expect(state.seenIds).toEqual(['t4']);
	});

	it('accumulates same-day IDs when the cursor does not advance', async () => {
		// Cursor already on 06-08; a new same-day row (t9) arrives, the date does not move.
		const state: Record<string, unknown> = { lastBookingDate: '2026-06-08', seenIds: ['t2', 't3'] };
		const wires = [wire('t2', '2026-06-08'), wire('t3', '2026-06-08'), wire('t9', '2026-06-08')];
		const { ctx } = makeContext('trigger', state, wires);

		const out = await OpenBankingIoTrigger.prototype.poll.call(ctx as never);

		expect(out?.[0]).toHaveLength(1);
		expect(out?.[0][0].json.id).toBe('t9');
		expect(state.lastBookingDate).toBe('2026-06-08');
		// Date held steady, so prior boundary IDs are kept and the new one is added (union).
		expect((state.seenIds as string[]).sort()).toEqual(['t2', 't3', 't9']);
	});

	it('returns a sample but does not mutate state on a manual test run', async () => {
		const state: Record<string, unknown> = { lastBookingDate: '2026-06-08' };
		const { ctx } = makeContext('manual', state, [wire('t1', '2026-06-08')]);

		const out = await OpenBankingIoTrigger.prototype.poll.call(ctx as never);

		expect(out?.[0]).toHaveLength(1); // still returns a sample for the test view
		expect(state.lastBookingDate).toBe('2026-06-08'); // but never advances the cursor
		expect(state.seenIds).toBeUndefined();
	});
});
