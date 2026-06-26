import { NodeApiError } from 'n8n-workflow';
import type {
	IDataObject,
	INodeExecutionData,
	INodeType,
	INodeTypeDescription,
	IPollFunctions,
	JsonObject,
} from 'n8n-workflow';

import {
	apiRequest,
	collectTransactionWires,
	importBundleKey,
	mapTransaction,
	resolveBundle,
	transactionsPath,
} from './shared/client';
import type { AccountWire, TransactionPageWire } from './shared/models';

interface TriggerState {
	lastBookingDate?: string;
	/**
	 * IDs already emitted that fall on `lastBookingDate` (or have no booking date yet). The next
	 * poll re-fetches from `lastBookingDate`, so these are the only rows it can return again — we
	 * keep exactly them to dedupe. Naturally bounded by one day's transactions, no arbitrary cap.
	 */
	seenIds?: string[];
}

export class OpenBankingIoTrigger implements INodeType {
	description: INodeTypeDescription = {
		displayName: 'Open Banking IO Trigger',
		name: 'openBankingIoTrigger',
		icon: 'file:openBankingIo.svg',
		group: ['trigger'],
		version: 1,
		subtitle: '=Poll for new transactions',
		description: 'Starts a workflow when new bank transactions arrive on open-banking.io',
		polling: true,
		defaults: {
			name: 'Open Banking IO Trigger',
		},
		inputs: [],
		outputs: ['main'],
		credentials: [
			{
				name: 'openBankingIoApi',
				required: true,
			},
		],
		properties: [
			{
				displayName: 'Initial Lookback (Days)',
				name: 'lookbackDays',
				type: 'number',
				typeOptions: { minValue: 1 },
				default: 7,
				description:
					'On the first poll, how many days back to fetch transactions from. Later polls only fetch transactions newer than the last one seen.',
			},
		],
	};

	async poll(this: IPollFunctions): Promise<INodeExecutionData[][] | null> {
		try {
			const bundle = resolveBundle(await this.getCredentials('openBankingIoApi'));
			const key = await importBundleKey(bundle);
			const state = this.getWorkflowStaticData('node') as TriggerState;
			const isManual = this.getMode() === 'manual';

			const lookbackDays = this.getNodeParameter('lookbackDays', 7) as number;
			const from = state.lastBookingDate ?? isoDaysAgo(lookbackDays);

			const accounts = await apiRequest<AccountWire[]>(this, bundle, 'GET', '/api/accounts');

			const seen = new Set(state.seenIds ?? []);
			const fresh: INodeExecutionData[] = [];
			// Track each emitted row's booking date so we can keep only the boundary-date IDs below.
			const emitted: Array<{ id: string; bookingDate: string | null }> = [];
			let maxBookingDate = from;

			for (const account of accounts) {
				const path = transactionsPath(account.id);
				const wires = await collectTransactionWires(
					(offset, limit) =>
						apiRequest<TransactionPageWire>(this, bundle, 'GET', path, { from, offset, limit }),
					Number.POSITIVE_INFINITY,
				);

				for (const wire of wires) {
					if (seen.has(wire.id)) continue;
					seen.add(wire.id);
					const bookingDate = wire.bookingDate ?? null;
					emitted.push({ id: wire.id, bookingDate });
					fresh.push({
						json: {
							accountId: account.id,
							...(await mapTransaction(key, wire)),
						} as unknown as IDataObject,
					});
					if (bookingDate && bookingDate > maxBookingDate) {
						maxBookingDate = bookingDate;
					}
				}
			}

			// A manual test run must not advance the cursor, so the user can re-test freely.
			if (!isManual) {
				state.lastBookingDate = maxBookingDate;
				// Only rows on the new boundary date (or undated/pending ones) can be re-fetched next
				// poll, so those are all we need to remember. If the date didn't advance, carry the
				// prior boundary IDs forward; otherwise the earlier date is now out of range — drop it.
				const boundaryIds = emitted
					.filter((e) => e.bookingDate == null || e.bookingDate === maxBookingDate)
					.map((e) => e.id);
				state.seenIds =
					maxBookingDate === from
						? [...new Set([...(state.seenIds ?? []), ...boundaryIds])]
						: boundaryIds;
			}

			return fresh.length ? [fresh] : null;
		} catch (error) {
			throw new NodeApiError(this.getNode(), error as JsonObject);
		}
	}
}

/** Returns the YYYY-MM-DD date `days` days before today (UTC). */
function isoDaysAgo(days: number): string {
	const d = new Date();
	d.setUTCDate(d.getUTCDate() - days);
	return d.toISOString().slice(0, 10);
}
