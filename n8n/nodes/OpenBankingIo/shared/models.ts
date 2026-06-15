// Self-contained copy of the open-banking.io domain + wire types. Duplicated here
// (rather than imported from @open-banking-io/client) so the package has zero
// runtime dependencies, as required for n8n community-node verification.
//
// Monetary amounts are decimal STRINGS throughout — never parse them to floats.

export interface Account {
	id: string;
	aspspName: string;
	aspspCountry: string;
	currency: string;
	accountType: string | null;
	bic: string | null;
	needsReconnect: boolean;

	iban: string | null;
	bban: string | null;
	ownerName: string | null;
	accountName: string | null;
	product: string | null;
	displayName: string | null;

	balances: Balance[];
}

export interface Balance {
	type: string;
	name: string | null;
	amount: string;
	currency: string;
	referenceDate: string | null;
}

export interface Transaction {
	id: string;
	currency: string;
	creditDebitIndicator: string;
	status: string | null;
	bookingDate: string | null;
	valueDate: string | null;
	transactionDate: string | null;
	bankTransactionCode: string | null;

	amount: string;
	creditorName: string | null;
	creditorIban: string | null;
	creditorBban: string | null;
	creditorAgentBic: string | null;
	debtorName: string | null;
	debtorIban: string | null;
	debtorBban: string | null;
	debtorAgentBic: string | null;
	remittanceInformation: string | null;
	note: string | null;
	referenceNumber: string | null;
	exchangeRate: string | null;
	merchantCategoryCode: string | null;
	balanceAfterTransaction: string | null;
	balanceAfterCurrency: string | null;
}

export interface TransactionPage {
	items: Transaction[];
	total: number;
}

export interface Connection {
	sessionId: string;
	aspspName: string;
	aspspCountry: string;
	validUntil: string;
	status: string;
	accountCount: number;
	lastSyncedAt: string | null;
	psuType: string | null;
}

export interface SyncResult {
	newTransactions: number;
	totalFetched: number;
}

export interface SyncAllResult {
	accounts: number;
	newTransactions: number;
}

export interface AccountWire {
	id: string;
	aspspName: string;
	aspspCountry: string;
	currency: string;
	accountType?: string | null;
	bic?: string | null;
	needsReconnect: boolean;
	balances: BalanceWire[];
	enc?: string | null;
	displayNameEnc?: string | null;
	uidEnc?: string | null;
}

export interface BalanceWire {
	type: string;
	currency: string;
	referenceDate?: string | null;
	enc?: string | null;
}

export interface TransactionPageWire {
	items: TransactionWire[];
	total: number;
}

export interface TransactionWire {
	id: string;
	currency: string;
	creditDebitIndicator: string;
	status?: string | null;
	bookingDate?: string | null;
	valueDate?: string | null;
	transactionDate?: string | null;
	bankTransactionCode?: string | null;
	enc?: string | null;
}

export interface ConnectionWire {
	sessionId: string;
	aspspName: string;
	aspspCountry: string;
	validUntil: string;
	status: string;
	accountCount: number;
	lastSyncedAt?: string | null;
	psuType?: string | null;
}

export interface SyncResultWire {
	newTransactions: number;
	totalFetched: number;
}

export interface SyncAllResultWire {
	accounts: number;
	newTransactions: number;
}

export interface Bank {
	name: string;
	country: string;
	[key: string]: unknown;
}

export interface AccountEnc {
	ownerName?: string | null;
	iban?: string | null;
	bban?: string | null;
	accountName?: string | null;
	product?: string | null;
}

export interface DisplayNameEnc {
	displayName?: string | null;
}

export interface UidEnc {
	uid?: string | null;
}

export interface BalanceEnc {
	amount?: string | null;
	name?: string | null;
}

export interface TransactionEnc {
	amount?: string | null;
	creditorName?: string | null;
	creditorIban?: string | null;
	creditorBban?: string | null;
	creditorAgentBic?: string | null;
	debtorName?: string | null;
	debtorIban?: string | null;
	debtorBban?: string | null;
	debtorAgentBic?: string | null;
	remittanceInformation?: string | null;
	note?: string | null;
	referenceNumber?: string | null;
	exchangeRate?: string | null;
	merchantCategoryCode?: string | null;
	balanceAfter?: string | null;
	balanceAfterCurrency?: string | null;
	rawJson?: string | null;
}
