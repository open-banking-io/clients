package openbanking

import (
	"bytes"
	"crypto/ecdh"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Client is a decrypting client for the open-banking.io API. Authenticates with an API key
// (X-Api-Key) and decrypts the zero-knowledge data envelopes locally with the exported private key.
type Client struct {
	baseURL    string
	apiKey     string
	privateKey *ecdh.PrivateKey
	httpClient *http.Client
}

// New builds a client from an API base url, API key, and base64 PKCS#8 encryption private key.
// A nil httpClient uses http.DefaultClient.
func New(apiBaseURL, apiKey, privateKeyPKCS8B64 string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(apiBaseURL) == "" {
		return nil, fmt.Errorf("apiBaseUrl is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("apiKey is required")
	}
	if strings.TrimSpace(privateKeyPKCS8B64) == "" {
		return nil, fmt.Errorf("privateKeyPkcs8 is required")
	}
	priv, err := loadPrivateKey(privateKeyPKCS8B64)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    strings.TrimRight(apiBaseURL, "/"),
		apiKey:     apiKey,
		privateKey: priv,
		httpClient: httpClient,
	}, nil
}

// FromCredentials builds a client from a credentials-bundle JSON string or a path to a bundle file.
func FromCredentials(pathOrJSON string, httpClient *http.Client) (*Client, error) {
	raw := pathOrJSON
	if !strings.HasPrefix(strings.TrimSpace(pathOrJSON), "{") {
		data, err := os.ReadFile(pathOrJSON)
		if err != nil {
			return nil, fmt.Errorf("could not read credentials file: %w", err)
		}
		raw = string(data)
	}
	var bundle CredentialsBundle
	if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
		return nil, fmt.Errorf("could not parse the credentials bundle: %w", err)
	}
	return FromBundle(bundle, httpClient)
}

// FromBundle builds a client from an already-parsed credentials bundle.
func FromBundle(bundle CredentialsBundle, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(bundle.APIKey) == "" {
		return nil, fmt.Errorf("the credentials bundle has no apiKey")
	}
	if strings.TrimSpace(bundle.EncryptionKey.PrivateKey) == "" {
		return nil, fmt.Errorf("the credentials bundle has no encryption private key")
	}
	return New(bundle.APIBaseURL, bundle.APIKey, bundle.EncryptionKey.PrivateKey, httpClient)
}

// GetAccounts lists the user's accounts with all sensitive fields decrypted.
func (c *Client) GetAccounts() ([]Account, error) {
	wires, err := c.getAccountWires()
	if err != nil {
		return nil, err
	}
	accounts := make([]Account, 0, len(wires))
	for i := range wires {
		a, err := c.mapAccount(&wires[i])
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// GetTransactions returns a page of an account's statement, newest first, with decrypted fields.
func (c *Client) GetTransactions(accountID string, query TransactionQuery) (TransactionPage, error) {
	params := url.Values{}
	if query.From != "" {
		params.Set("from", query.From)
	}
	if query.To != "" {
		params.Set("to", query.To)
	}
	if query.Limit != nil {
		params.Set("limit", strconv.Itoa(*query.Limit))
	}
	if query.Offset != nil {
		params.Set("offset", strconv.Itoa(*query.Offset))
	}
	path := "/api/accounts/" + url.PathEscape(accountID) + "/transactions"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var page transactionPageWire
	if err := c.getJSON(path, &page); err != nil {
		return TransactionPage{}, err
	}
	items := make([]Transaction, 0, len(page.Items))
	for i := range page.Items {
		t, err := c.mapTransaction(&page.Items[i])
		if err != nil {
			return TransactionPage{}, err
		}
		items = append(items, t)
	}
	return TransactionPage{Items: items, Total: page.Total}, nil
}

// GetConnections lists the user's bank connections.
func (c *Client) GetConnections() ([]Connection, error) {
	var wires []connectionWire
	if err := c.getJSON("/api/connections", &wires); err != nil {
		return nil, err
	}
	conns := make([]Connection, 0, len(wires))
	for _, w := range wires {
		conns = append(conns, Connection{
			SessionID:    w.SessionID,
			AspspName:    w.AspspName,
			AspspCountry: w.AspspCountry,
			ValidUntil:   w.ValidUntil,
			Status:       w.Status,
			AccountCount: w.AccountCount,
			LastSyncedAt: w.LastSyncedAt,
			PsuType:      w.PsuType,
		})
	}
	return conns, nil
}

// Sync triggers an online sync of one account: decrypts that account's Enable Banking uid and posts
// it, so the service can fetch fresh data without ever holding the uid in plaintext.
func (c *Client) Sync(accountID string) (SyncResult, error) {
	wires, err := c.getAccountWires()
	if err != nil {
		return SyncResult{}, err
	}
	var account *accountWire
	for i := range wires {
		if wires[i].ID == accountID {
			account = &wires[i]
			break
		}
	}
	if account == nil {
		return SyncResult{}, fmt.Errorf("account %s not found", accountID)
	}
	uid, err := c.decryptUid(account)
	if err != nil {
		return SyncResult{}, err
	}
	if uid == "" {
		return SyncResult{}, fmt.Errorf("account has no active session (reconnect required) — cannot sync")
	}

	var result syncResultWire
	if err := c.postJSON("/api/accounts/"+url.PathEscape(accountID)+"/sync",
		map[string]any{"uid": uid}, &result); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{NewTransactions: result.NewTransactions, TotalFetched: result.TotalFetched}, nil
}

// SyncAll triggers an online sync of every account that has an active session.
func (c *Client) SyncAll() (SyncAllResult, error) {
	wires, err := c.getAccountWires()
	if err != nil {
		return SyncAllResult{}, err
	}
	items := make([]map[string]string, 0, len(wires))
	for i := range wires {
		uid, err := c.decryptUid(&wires[i])
		if err != nil {
			return SyncAllResult{}, err
		}
		if uid != "" {
			items = append(items, map[string]string{"accountId": wires[i].ID, "uid": uid})
		}
	}

	var result syncAllResultWire
	if err := c.postJSON("/api/sync", map[string]any{"items": items}, &result); err != nil {
		return SyncAllResult{}, err
	}
	return SyncAllResult{Accounts: result.Accounts, NewTransactions: result.NewTransactions}, nil
}

// ---- internals ----

func (c *Client) getAccountWires() ([]accountWire, error) {
	var wires []accountWire
	if err := c.getJSON("/api/accounts", &wires); err != nil {
		return nil, err
	}
	return wires, nil
}

func (c *Client) decryptUid(a *accountWire) (string, error) {
	var dec uidEnc
	if _, err := decryptTo(c.privateKey, a.UidEnc, &dec); err != nil {
		return "", err
	}
	return dec.Uid, nil
}

func (c *Client) mapAccount(a *accountWire) (Account, error) {
	var acc accountEnc
	if _, err := decryptTo(c.privateKey, a.Enc, &acc); err != nil {
		return Account{}, err
	}
	var name displayNameEnc
	if _, err := decryptTo(c.privateKey, a.DisplayNameEnc, &name); err != nil {
		return Account{}, err
	}

	balances := make([]Balance, 0, len(a.Balances))
	for _, b := range a.Balances {
		var dec balanceEnc
		if _, err := decryptTo(c.privateKey, b.Enc, &dec); err != nil {
			return Account{}, err
		}
		amount := dec.Amount
		if amount == "" {
			amount = "0"
		}
		balances = append(balances, Balance{
			Type:          b.Type,
			Name:          dec.Name,
			Amount:        amount,
			Currency:      b.Currency,
			ReferenceDate: b.ReferenceDate,
		})
	}

	return Account{
		ID:             a.ID,
		AspspName:      a.AspspName,
		AspspCountry:   a.AspspCountry,
		Currency:       a.Currency,
		AccountType:    a.AccountType,
		Bic:            a.Bic,
		NeedsReconnect: a.NeedsReconnect,
		Iban:           acc.Iban,
		Bban:           acc.Bban,
		OwnerName:      acc.OwnerName,
		AccountName:    acc.AccountName,
		Product:        acc.Product,
		DisplayName:    name.DisplayName,
		Balances:       balances,
	}, nil
}

func (c *Client) mapTransaction(t *transactionWire) (Transaction, error) {
	var d transactionEnc
	if _, err := decryptTo(c.privateKey, t.Enc, &d); err != nil {
		return Transaction{}, err
	}
	amount := d.Amount
	if amount == "" {
		amount = "0"
	}
	return Transaction{
		ID:                      t.ID,
		Currency:                t.Currency,
		CreditDebitIndicator:    t.CreditDebitIndicator,
		Status:                  t.Status,
		BookingDate:             t.BookingDate,
		ValueDate:               t.ValueDate,
		TransactionDate:         t.TransactionDate,
		BankTransactionCode:     t.BankTransactionCode,
		Amount:                  amount,
		CreditorName:            d.CreditorName,
		CreditorIban:            d.CreditorIban,
		CreditorBban:            d.CreditorBban,
		CreditorAgentBic:        d.CreditorAgentBic,
		DebtorName:              d.DebtorName,
		DebtorIban:              d.DebtorIban,
		DebtorBban:              d.DebtorBban,
		DebtorAgentBic:          d.DebtorAgentBic,
		RemittanceInformation:   d.RemittanceInformation,
		Note:                    d.Note,
		ReferenceNumber:         d.ReferenceNumber,
		ExchangeRate:            d.ExchangeRate,
		MerchantCategoryCode:    d.MerchantCategoryCode,
		BalanceAfterTransaction: d.BalanceAfter,
		BalanceAfterCurrency:    d.BalanceAfterCurrency,
	}, nil
}

func (c *Client) getJSON(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, path, out)
}

func (c *Client) postJSON(path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, path, out)
}

func (c *Client) do(req *http.Request, path string, out any) error {
	req.Header.Set("X-Api-Key", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s failed: %w", req.Method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s failed: %d %s", req.Method, path, resp.StatusCode, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s %s read failed: %w", req.Method, path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s %s decode failed: %w", req.Method, path, err)
	}
	return nil
}
