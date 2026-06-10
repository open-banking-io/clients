package openbanking

// Public, decrypted models. Optional text fields are empty strings when absent. Monetary amounts
// are kept as the string the service emits, so no precision is lost in a float round-trip — parse
// them with your decimal type of choice.

// Balance is a balance snapshot. Type is the ISO 20022 code (ITBD booked, ITAV available, ...).
type Balance struct {
	Type          string
	Name          string
	Amount        string
	Currency      string
	ReferenceDate string
}

// Account is a bank account with its sensitive fields decrypted.
type Account struct {
	ID             string
	AspspName      string
	AspspCountry   string
	Currency       string
	AccountType    string
	Bic            string
	NeedsReconnect bool
	Iban           string
	Bban           string
	OwnerName      string
	AccountName    string
	Product        string
	DisplayName    string
	Balances       []Balance
}

// Transaction is a statement transaction with its sensitive fields decrypted.
type Transaction struct {
	ID                      string
	Currency                string
	CreditDebitIndicator    string
	Status                  string
	BookingDate             string
	ValueDate               string
	TransactionDate         string
	BankTransactionCode     string
	Amount                  string
	CreditorName            string
	CreditorIban            string
	CreditorBban            string
	CreditorAgentBic        string
	DebtorName              string
	DebtorIban              string
	DebtorBban              string
	DebtorAgentBic          string
	RemittanceInformation   string
	Note                    string
	ReferenceNumber         string
	ExchangeRate            string
	MerchantCategoryCode    string
	BalanceAfterTransaction string
	BalanceAfterCurrency    string
}

// TransactionPage is a page of transactions, newest first.
type TransactionPage struct {
	Items []Transaction
	Total int64
}

// TransactionQuery holds the optional filters for a transactions page.
type TransactionQuery struct {
	From   string
	To     string
	Limit  *int
	Offset *int
}

// Connection is a bank connection (consent).
type Connection struct {
	SessionID    string
	AspspName    string
	AspspCountry string
	ValidUntil   string
	Status       string
	AccountCount int64
	LastSyncedAt string
	PsuType      string
}

// SyncResult is the outcome of syncing one account.
type SyncResult struct {
	NewTransactions int64
	TotalFetched    int64
}

// SyncAllResult is the outcome of syncing every account with an active session.
type SyncAllResult struct {
	Accounts        int64
	NewTransactions int64
}

// CredentialsBundle is the bundle exported from open-banking.io (API key + encryption private key).
type CredentialsBundle struct {
	Service       string        `json:"service"`
	APIBaseURL    string        `json:"apiBaseUrl"`
	User          string        `json:"user"`
	APIKey        string        `json:"apiKey"`
	EncryptionKey EncryptionKey `json:"encryptionKey"`
}

// EncryptionKey holds the base64 PKCS#8 encryption private key from a credentials bundle.
type EncryptionKey struct {
	Scheme           string `json:"scheme"`
	Curve            string `json:"curve"`
	PrivateKeyFormat string `json:"privateKeyFormat"`
	PrivateKey       string `json:"privateKey"`
	PublicKey        string `json:"publicKey"`
}

// ---- Internal wire DTOs (what the API returns; sensitive fields are ciphertext) ----

type accountWire struct {
	ID             string        `json:"id"`
	AspspName      string        `json:"aspspName"`
	AspspCountry   string        `json:"aspspCountry"`
	Currency       string        `json:"currency"`
	AccountType    string        `json:"accountType"`
	Bic            string        `json:"bic"`
	NeedsReconnect bool          `json:"needsReconnect"`
	Balances       []balanceWire `json:"balances"`
	Enc            string        `json:"enc"`
	DisplayNameEnc string        `json:"displayNameEnc"`
	UidEnc         string        `json:"uidEnc"`
}

type balanceWire struct {
	Type          string `json:"type"`
	Currency      string `json:"currency"`
	ReferenceDate string `json:"referenceDate"`
	Enc           string `json:"enc"`
}

type transactionPageWire struct {
	Items []transactionWire `json:"items"`
	Total int64             `json:"total"`
}

type transactionWire struct {
	ID                   string `json:"id"`
	Currency             string `json:"currency"`
	CreditDebitIndicator string `json:"creditDebitIndicator"`
	Status               string `json:"status"`
	BookingDate          string `json:"bookingDate"`
	ValueDate            string `json:"valueDate"`
	TransactionDate      string `json:"transactionDate"`
	BankTransactionCode  string `json:"bankTransactionCode"`
	Enc                  string `json:"enc"`
}

type connectionWire struct {
	SessionID    string `json:"sessionId"`
	AspspName    string `json:"aspspName"`
	AspspCountry string `json:"aspspCountry"`
	ValidUntil   string `json:"validUntil"`
	Status       string `json:"status"`
	AccountCount int64  `json:"accountCount"`
	LastSyncedAt string `json:"lastSyncedAt"`
	PsuType      string `json:"psuType"`
}

type syncResultWire struct {
	NewTransactions int64 `json:"newTransactions"`
	TotalFetched    int64 `json:"totalFetched"`
}

type syncAllResultWire struct {
	Accounts        int64 `json:"accounts"`
	NewTransactions int64 `json:"newTransactions"`
}

// ---- Decrypted envelope payloads (the camelCase contract with the backend) ----

type accountEnc struct {
	OwnerName   string `json:"ownerName"`
	Iban        string `json:"iban"`
	Bban        string `json:"bban"`
	AccountName string `json:"accountName"`
	Product     string `json:"product"`
}

type displayNameEnc struct {
	DisplayName string `json:"displayName"`
}

type uidEnc struct {
	Uid string `json:"uid"`
}

type balanceEnc struct {
	Amount string `json:"amount"`
	Name   string `json:"name"`
}

type transactionEnc struct {
	Amount                string `json:"amount"`
	CreditorName          string `json:"creditorName"`
	CreditorIban          string `json:"creditorIban"`
	CreditorBban          string `json:"creditorBban"`
	CreditorAgentBic      string `json:"creditorAgentBic"`
	DebtorName            string `json:"debtorName"`
	DebtorIban            string `json:"debtorIban"`
	DebtorBban            string `json:"debtorBban"`
	DebtorAgentBic        string `json:"debtorAgentBic"`
	RemittanceInformation string `json:"remittanceInformation"`
	Note                  string `json:"note"`
	ReferenceNumber       string `json:"referenceNumber"`
	ExchangeRate          string `json:"exchangeRate"`
	MerchantCategoryCode  string `json:"merchantCategoryCode"`
	BalanceAfter          string `json:"balanceAfter"`
	BalanceAfterCurrency  string `json:"balanceAfterCurrency"`
}
