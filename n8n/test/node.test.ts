import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { describe, expect, it, vi } from 'vitest';

import { OpenBankingIo } from '../nodes/OpenBankingIo/OpenBankingIo.node';

// Drives the real node `execute()` with a faked n8n context that serves the shared API fixtures.
// This exercises the full path — parameter dispatch, the HTTP helper call, pagination, and local
// envelope decryption — without a network or an n8n install.

const FIXTURES = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'fixtures');

function fixture(rel: string): unknown {
	return JSON.parse(readFileSync(join(FIXTURES, rel), 'utf8'));
}

const BUNDLE = readFileSync(join(FIXTURES, 'credentials.json'), 'utf8');

/** Builds a minimal IExecuteFunctions stand-in for one input item. */
function makeContext(params: Record<string, unknown>, routes: Record<string, unknown>) {
	const httpRequestWithAuthentication = vi.fn(async (_cred: string, options: { url: string }) => {
		const path = options.url.replace('http://localhost:8081', '');
		for (const [prefix, body] of Object.entries(routes)) {
			if (path.startsWith(prefix)) return body;
		}
		throw new Error(`unexpected request: ${path}`);
	});

	const ctx = {
		getInputData: () => [{ json: {} }],
		getCredentials: async () => ({ bundle: BUNDLE }),
		getNodeParameter: (name: string, _i: number, fallback?: unknown) =>
			name in params ? params[name] : fallback,
		continueOnFail: () => false,
		getNode: () => ({ name: 'Open Banking IO' }),
		helpers: { httpRequestWithAuthentication },
	};

	return { ctx, httpRequestWithAuthentication };
}

describe('OpenBankingIo node execute()', () => {
	it('lists and decrypts accounts', async () => {
		const { ctx } = makeContext(
			{ resource: 'account', operation: 'getMany' },
			{ '/api/accounts': fixture('api/accounts.json') },
		);

		const [out] = await OpenBankingIo.prototype.execute.call(ctx as never);

		expect(out).toHaveLength(1);
		const account = out[0].json;
		expect(account.ownerName).toBe('Tatic ApS');
		expect(account.iban).toBe('DK6466952001724927');
		expect(account.displayName).toBe('Drift');
		// Decimal amounts stay strings, in the wire's balance order (ITBD, ITAV, OTHR).
		expect(account.balances).toMatchObject([
			{ type: 'ITBD', amount: '828.13' },
			{ type: 'ITAV', amount: '633.90' },
			{ type: 'OTHR', amount: '194.23' },
		]);
	});

	it('fetches and decrypts a transaction statement', async () => {
		const accountId = '11111111-1111-4111-8111-111111111111';
		const { ctx, httpRequestWithAuthentication } = makeContext(
			{ resource: 'transaction', operation: 'getMany', accountId, returnAll: true, filters: {} },
			{ [`/api/accounts/${accountId}/transactions`]: fixture('api/transactions.json') },
		);

		const [out] = await OpenBankingIo.prototype.execute.call(ctx as never);

		expect(out).toHaveLength(1);
		const txn = out[0].json;
		expect(txn.accountId).toBe(accountId);
		expect(txn.amount).toBe('194.23');
		expect(txn.creditorName).toBe('One.com');
		expect(txn.merchantCategoryCode).toBe('4816');
		expect(txn.balanceAfterTransaction).toBe('633.90');
		// One page was enough (total === items.length), so we stop after a single request.
		expect(httpRequestWithAuthentication).toHaveBeenCalledTimes(1);
	});

	it('surfaces a clear error for an invalid credentials bundle', async () => {
		const httpRequestWithAuthentication = vi.fn();
		const ctx = {
			getInputData: () => [{ json: {} }],
			getCredentials: async () => ({ bundle: 'not json' }),
			getNodeParameter: () => undefined,
			continueOnFail: () => false,
			getNode: () => ({ name: 'Open Banking IO' }),
			helpers: { httpRequestWithAuthentication },
		};

		await expect(OpenBankingIo.prototype.execute.call(ctx as never)).rejects.toThrow(
			/not valid JSON/,
		);
		expect(httpRequestWithAuthentication).not.toHaveBeenCalled();
	});
});
