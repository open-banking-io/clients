import {
	NodeOperationError,
	type IDataObject,
	type IExecuteFunctions,
	type INodeExecutionData,
	type INodeType,
	type INodeTypeDescription,
} from 'n8n-workflow';

import {
	apiRequest,
	collectTransactionWires,
	decryptUid,
	importBundleKey,
	mapAccount,
	mapConnection,
	mapTransaction,
	parseBundle,
	transactionsPath,
	type Bundle,
} from './shared/client';
import type {
	AccountWire,
	Bank,
	ConnectionWire,
	SyncAllResultWire,
	SyncResultWire,
	TransactionPageWire,
} from './shared/models';

export class OpenBankingIo implements INodeType {
	description: INodeTypeDescription = {
		displayName: 'Open Banking IO',
		name: 'openBankingIo',
		icon: 'file:openBankingIo.svg',
		group: ['transform'],
		version: 1,
		subtitle: '={{ $parameter["operation"] + ": " + $parameter["resource"] }}',
		description: 'Read bank accounts, balances and transactions from open-banking.io',
		defaults: {
			name: 'Open Banking IO',
		},
		inputs: ['main'],
		outputs: ['main'],
		credentials: [
			{
				name: 'openBankingIoApi',
				required: true,
			},
		],
		properties: [
			{
				displayName: 'Resource',
				name: 'resource',
				type: 'options',
				noDataExpression: true,
				options: [
					{ name: 'Account', value: 'account' },
					{ name: 'Bank', value: 'bank' },
					{ name: 'Connection', value: 'connection' },
					{ name: 'Sync', value: 'sync' },
					{ name: 'Transaction', value: 'transaction' },
				],
				default: 'transaction',
			},

			// ---- Account ----
			{
				displayName: 'Operation',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				displayOptions: { show: { resource: ['account'] } },
				options: [
					{
						name: 'Get Many',
						value: 'getMany',
						action: 'Get many accounts',
						description: 'List all accounts with decrypted details and balances',
					},
				],
				default: 'getMany',
			},

			// ---- Transaction ----
			{
				displayName: 'Operation',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				displayOptions: { show: { resource: ['transaction'] } },
				options: [
					{
						name: 'Get Many',
						value: 'getMany',
						action: 'Get many transactions',
						description: 'List an account statement with decrypted details',
					},
				],
				default: 'getMany',
			},
			{
				displayName: 'Account ID',
				name: 'accountId',
				type: 'string',
				required: true,
				default: '',
				displayOptions: { show: { resource: ['transaction'], operation: ['getMany'] } },
				description: 'The account whose transactions to fetch',
			},
			{
				displayName: 'Return All',
				name: 'returnAll',
				type: 'boolean',
				default: false,
				displayOptions: { show: { resource: ['transaction'], operation: ['getMany'] } },
				description: 'Whether to return all results or only up to a given limit',
			},
			{
				displayName: 'Limit',
				name: 'limit',
				type: 'number',
				typeOptions: { minValue: 1 },
				default: 50,
				displayOptions: {
					show: { resource: ['transaction'], operation: ['getMany'], returnAll: [false] },
				},
				description: 'Max number of results to return',
			},
			{
				displayName: 'Filters',
				name: 'filters',
				type: 'collection',
				placeholder: 'Add Filter',
				default: {},
				displayOptions: { show: { resource: ['transaction'], operation: ['getMany'] } },
				options: [
					{
						displayName: 'From Date',
						name: 'from',
						type: 'string',
						default: '',
						placeholder: 'YYYY-MM-DD',
						description: 'Only transactions booked on or after this date',
					},
					{
						displayName: 'To Date',
						name: 'to',
						type: 'string',
						default: '',
						placeholder: 'YYYY-MM-DD',
						description: 'Only transactions booked on or before this date',
					},
				],
			},

			// ---- Connection ----
			{
				displayName: 'Operation',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				displayOptions: { show: { resource: ['connection'] } },
				options: [
					{
						name: 'Get Many',
						value: 'getMany',
						action: 'Get many connections',
						description: 'List bank connections (consents)',
					},
				],
				default: 'getMany',
			},

			// ---- Bank ----
			{
				displayName: 'Operation',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				displayOptions: { show: { resource: ['bank'] } },
				options: [
					{
						name: 'Get Many',
						value: 'getMany',
						action: 'Get many banks',
						description: 'List available banks (ASPSPs)',
					},
				],
				default: 'getMany',
			},
			{
				displayName: 'Country',
				name: 'country',
				type: 'string',
				default: '',
				placeholder: 'DK',
				displayOptions: { show: { resource: ['bank'], operation: ['getMany'] } },
				description: 'Optional ISO 3166 country code to filter banks by',
			},

			// ---- Sync ----
			{
				displayName: 'Operation',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				displayOptions: { show: { resource: ['sync'] } },
				options: [
					{
						name: 'Sync Account',
						value: 'account',
						action: 'Sync an account',
						description: 'Pull fresh transactions for one account from the bank',
					},
					{
						name: 'Sync All',
						value: 'all',
						action: 'Sync all accounts',
						description: 'Pull fresh transactions for every account with an active session',
					},
				],
				default: 'account',
			},
			{
				displayName: 'Account ID',
				name: 'accountId',
				type: 'string',
				required: true,
				default: '',
				displayOptions: { show: { resource: ['sync'], operation: ['account'] } },
				description: 'The account to sync',
			},
		],
	};

	async execute(this: IExecuteFunctions): Promise<INodeExecutionData[][]> {
		const items = this.getInputData();
		const bundle = parseBundle((await this.getCredentials('openBankingIoApi')).bundle as string);
		const key = await importBundleKey(bundle);
		const out: INodeExecutionData[] = [];

		for (let i = 0; i < items.length; i++) {
			try {
				const resource = this.getNodeParameter('resource', i) as string;
				const operation = this.getNodeParameter('operation', i) as string;

				if (resource === 'account' && operation === 'getMany') {
					const wires = await apiRequest<AccountWire[]>(this, bundle, 'GET', '/api/accounts');
					for (const w of wires) {
						out.push({ json: (await mapAccount(key, w)) as unknown as IDataObject, pairedItem: i });
					}
				} else if (resource === 'transaction' && operation === 'getMany') {
					await fetchTransactions.call(this, bundle, key, i, out);
				} else if (resource === 'connection' && operation === 'getMany') {
					const wires = await apiRequest<ConnectionWire[]>(this, bundle, 'GET', '/api/connections');
					for (const w of wires) {
						out.push({ json: mapConnection(w) as unknown as IDataObject, pairedItem: i });
					}
				} else if (resource === 'bank' && operation === 'getMany') {
					const country = (this.getNodeParameter('country', i) as string).trim();
					const qs = country ? { country } : undefined;
					const banks = await apiRequest<Bank[]>(this, bundle, 'GET', '/api/aspsps', qs);
					for (const b of banks) {
						out.push({ json: b as unknown as IDataObject, pairedItem: i });
					}
				} else if (resource === 'sync' && operation === 'account') {
					out.push({ json: await syncAccount.call(this, bundle, key, i), pairedItem: i });
				} else if (resource === 'sync' && operation === 'all') {
					out.push({ json: await syncAll.call(this, bundle, key), pairedItem: i });
				} else {
					throw new NodeOperationError(
						this.getNode(),
						`Unsupported operation "${operation}" for resource "${resource}"`,
						{ itemIndex: i },
					);
				}
			} catch (error) {
				if (this.continueOnFail()) {
					out.push({ json: { error: (error as Error).message }, pairedItem: i });
					continue;
				}
				throw error;
			}
		}

		return [out];
	}
}

async function fetchTransactions(
	this: IExecuteFunctions,
	bundle: Bundle,
	key: Awaited<ReturnType<typeof importBundleKey>>,
	i: number,
	out: INodeExecutionData[],
): Promise<void> {
	const accountId = this.getNodeParameter('accountId', i) as string;
	const returnAll = this.getNodeParameter('returnAll', i) as boolean;
	const userLimit = returnAll
		? Number.POSITIVE_INFINITY
		: (this.getNodeParameter('limit', i) as number);
	const filters = this.getNodeParameter('filters', i, {}) as { from?: string; to?: string };

	const baseQs: IDataObject = {};
	if (filters.from) baseQs.from = filters.from;
	if (filters.to) baseQs.to = filters.to;

	const path = transactionsPath(accountId);
	const wires = await collectTransactionWires(
		(offset, limit) =>
			apiRequest<TransactionPageWire>(this, bundle, 'GET', path, {
				...baseQs,
				offset,
				limit,
			}),
		userLimit,
	);

	for (const w of wires) {
		out.push({
			json: { accountId, ...(await mapTransaction(key, w)) } as unknown as IDataObject,
			pairedItem: i,
		});
	}
}

async function syncAccount(
	this: IExecuteFunctions,
	bundle: Bundle,
	key: Awaited<ReturnType<typeof importBundleKey>>,
	i: number,
): Promise<IDataObject> {
	const accountId = this.getNodeParameter('accountId', i) as string;
	const wires = await apiRequest<AccountWire[]>(this, bundle, 'GET', '/api/accounts');
	const account = wires.find((a) => a.id === accountId);
	if (!account) {
		throw new NodeOperationError(this.getNode(), `Account ${accountId} not found`, {
			itemIndex: i,
		});
	}

	const uid = await decryptUid(key, account);
	if (uid == null) {
		throw new NodeOperationError(
			this.getNode(),
			'Account has no active session (reconnect required) — cannot sync',
			{ itemIndex: i },
		);
	}

	const result = await apiRequest<SyncResultWire>(
		this,
		bundle,
		'POST',
		`/api/accounts/${encodeURIComponent(accountId)}/sync`,
		undefined,
		{ uid },
	);
	return { newTransactions: result.newTransactions, totalFetched: result.totalFetched };
}

async function syncAll(
	this: IExecuteFunctions,
	bundle: Bundle,
	key: Awaited<ReturnType<typeof importBundleKey>>,
): Promise<IDataObject> {
	const wires = await apiRequest<AccountWire[]>(this, bundle, 'GET', '/api/accounts');
	const decrypted = await Promise.all(
		wires.map(async (a) => ({ accountId: a.id, uid: await decryptUid(key, a) })),
	);
	const syncItems = decrypted
		.filter((x): x is { accountId: string; uid: string } => x.uid != null)
		.map((x) => ({ accountId: x.accountId, uid: x.uid }));

	const result = await apiRequest<SyncAllResultWire>(this, bundle, 'POST', '/api/sync', undefined, {
		items: syncItems,
	});
	return { accounts: result.accounts, newTransactions: result.newTransactions };
}
