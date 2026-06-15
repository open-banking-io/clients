import type {
	IDataObject,
	IExecuteFunctions,
	IHttpRequestMethods,
	IHttpRequestOptions,
	ILoadOptionsFunctions,
	IPollFunctions,
} from 'n8n-workflow';

import { decryptTo, importPrivateKey, type PrivateKey } from './envelope';
import type {
	Account,
	AccountEnc,
	AccountWire,
	BalanceEnc,
	Connection,
	ConnectionWire,
	DisplayNameEnc,
	Transaction,
	TransactionEnc,
	TransactionPageWire,
	TransactionWire,
	UidEnc,
} from './models';

export const CREDENTIALS_NAME = 'openBankingIoApi';

/** The fields we need out of the pasted credentials bundle. */
export interface Bundle {
	apiBaseUrl: string;
	apiKey: string;
	privateKey: string;
}

/** n8n execution contexts that expose `helpers.httpRequestWithAuthentication`. */
export type RequestContext = IExecuteFunctions | IPollFunctions | ILoadOptionsFunctions;

/** Parses and validates the credentials-bundle JSON the user pasted into the credential. */
export function parseBundle(raw: string): Bundle {
	let parsed: IDataObject;
	try {
		parsed = JSON.parse(raw) as IDataObject;
	} catch {
		throw new Error('Credentials bundle is not valid JSON');
	}

	const apiBaseUrl = String((parsed.apiBaseUrl as string) ?? '').replace(/\/+$/, '');
	const apiKey = (parsed.apiKey as string) ?? '';
	const encryptionKey = (parsed.encryptionKey as IDataObject) ?? {};
	const privateKey = (encryptionKey.privateKey as string) ?? '';

	if (!apiBaseUrl) throw new Error('Credentials bundle is missing "apiBaseUrl"');
	if (!apiKey) throw new Error('Credentials bundle is missing "apiKey"');
	if (!privateKey) throw new Error('Credentials bundle is missing "encryptionKey.privateKey"');

	return { apiBaseUrl, apiKey, privateKey };
}

/**
 * Issues an authenticated request through n8n's HTTP helper. The `X-Api-Key` header is injected
 * by the credential's `authenticate` block, so we never put the key on the wire ourselves.
 */
export async function apiRequest<T>(
	ctx: RequestContext,
	bundle: Bundle,
	method: IHttpRequestMethods,
	path: string,
	qs?: IDataObject,
	body?: IDataObject,
): Promise<T> {
	// Coerce every query value to a string so numeric params (offset/limit) serialize
	// consistently, matching the string params (from/to/country) the callers pass.
	const stringQs = qs
		? Object.fromEntries(Object.entries(qs).map(([k, v]) => [k, String(v)]))
		: undefined;

	const options: IHttpRequestOptions = {
		method,
		url: bundle.apiBaseUrl + path,
		qs: stringQs,
		body,
		json: true,
	};
	return ctx.helpers.httpRequestWithAuthentication.call(
		ctx,
		CREDENTIALS_NAME,
		options,
	) as Promise<T>;
}

/** Imports the bundle's private key once for reuse across all rows of an execution. */
export function importBundleKey(bundle: Bundle): Promise<PrivateKey> {
	return importPrivateKey(bundle.privateKey);
}

export async function mapAccount(key: PrivateKey, a: AccountWire): Promise<Account> {
	const acc = await decryptTo<AccountEnc>(key, a.enc);
	const name = await decryptTo<DisplayNameEnc>(key, a.displayNameEnc);
	const balances = await Promise.all(
		(a.balances ?? []).map(async (b) => {
			const dec = await decryptTo<BalanceEnc>(key, b.enc);
			return {
				type: b.type,
				name: dec?.name ?? null,
				amount: dec?.amount ?? '0',
				currency: b.currency,
				referenceDate: b.referenceDate ?? null,
			};
		}),
	);

	return {
		id: a.id,
		aspspName: a.aspspName,
		aspspCountry: a.aspspCountry,
		currency: a.currency,
		accountType: a.accountType ?? null,
		bic: a.bic ?? null,
		needsReconnect: a.needsReconnect,
		iban: acc?.iban ?? null,
		bban: acc?.bban ?? null,
		ownerName: acc?.ownerName ?? null,
		accountName: acc?.accountName ?? null,
		product: acc?.product ?? null,
		displayName: name?.displayName ?? null,
		balances,
	};
}

export async function mapTransaction(key: PrivateKey, t: TransactionWire): Promise<Transaction> {
	const d = await decryptTo<TransactionEnc>(key, t.enc);
	return {
		id: t.id,
		currency: t.currency,
		creditDebitIndicator: t.creditDebitIndicator,
		status: t.status ?? null,
		bookingDate: t.bookingDate ?? null,
		valueDate: t.valueDate ?? null,
		transactionDate: t.transactionDate ?? null,
		bankTransactionCode: t.bankTransactionCode ?? null,
		amount: d?.amount ?? '0',
		creditorName: d?.creditorName ?? null,
		creditorIban: d?.creditorIban ?? null,
		creditorBban: d?.creditorBban ?? null,
		creditorAgentBic: d?.creditorAgentBic ?? null,
		debtorName: d?.debtorName ?? null,
		debtorIban: d?.debtorIban ?? null,
		debtorBban: d?.debtorBban ?? null,
		debtorAgentBic: d?.debtorAgentBic ?? null,
		remittanceInformation: d?.remittanceInformation ?? null,
		note: d?.note ?? null,
		referenceNumber: d?.referenceNumber ?? null,
		exchangeRate: d?.exchangeRate ?? null,
		merchantCategoryCode: d?.merchantCategoryCode ?? null,
		balanceAfterTransaction: d?.balanceAfter ?? null,
		balanceAfterCurrency: d?.balanceAfterCurrency ?? null,
	};
}

export function mapConnection(c: ConnectionWire): Connection {
	return {
		sessionId: c.sessionId,
		aspspName: c.aspspName,
		aspspCountry: c.aspspCountry,
		validUntil: c.validUntil,
		status: c.status,
		accountCount: c.accountCount,
		lastSyncedAt: c.lastSyncedAt ?? null,
		psuType: c.psuType ?? null,
	};
}

/** Decrypts an account's Enable Banking session uid, or returns null if there's no active session. */
export async function decryptUid(key: PrivateKey, a: AccountWire): Promise<string | null> {
	const dec = await decryptTo<UidEnc>(key, a.uidEnc);
	return dec?.uid ?? null;
}

/**
 * Collects transaction wires by looping the offset/limit window until the server's reported total
 * is reached, a page comes back empty, or `userLimit` rows are gathered. Pure (the page fetch is
 * injected) so it can be unit-tested without an n8n context. Decryption is the caller's job and
 * happens only on the rows actually returned.
 */
export async function collectTransactionWires(
	fetchPage: (offset: number, limit: number) => Promise<TransactionPageWire>,
	userLimit: number,
	pageSize = 100,
): Promise<TransactionWire[]> {
	const collected: TransactionWire[] = [];
	let offset = 0;
	let total = Number.POSITIVE_INFINITY;

	while (collected.length < Math.min(userLimit, total)) {
		const remaining = userLimit - collected.length;
		const limit = Math.min(pageSize, remaining);
		const page = await fetchPage(offset, limit);
		total = page.total;
		const items = page.items ?? [];
		if (items.length === 0) break;
		collected.push(...items);
		offset += items.length;
		if (collected.length >= total) break;
	}

	return collected.length > userLimit ? collected.slice(0, userLimit) : collected;
}

/** Fetches one page of an account's transactions. */
export function transactionsPath(accountId: string): string {
	return `/api/accounts/${encodeURIComponent(accountId)}/transactions`;
}
