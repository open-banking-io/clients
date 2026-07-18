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
	"time"
)

// Version is the released version of this client. It should track the release tag.
const Version = "0.3.0"

// Client is a decrypting client for the open-banking.io API. Authenticates with an API key
// (X-Api-Key) and decrypts the zero-knowledge data envelopes locally with the exported private key.
type Client struct {
	baseURL    string
	apiKey     string
	privateKey *ecdh.PrivateKey
	httpClient *http.Client
}

// New builds a client from an API base url, API key, and base64 PKCS#8 encryption private key.
// A nil httpClient builds an *http.Client with a 30s timeout.
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
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    strings.TrimRight(apiBaseURL, "/"),
		apiKey:     apiKey,
		privateKey: priv,
		httpClient: httpClient,
	}, nil
}

// optionState holds the mutable HTTP client an Option chain builds up. Options are applied in
// the order the caller passed them, each mutating this state, so last-applied wins across all
// option types.
type optionState struct {
	client *http.Client
}

// ensure returns the current *http.Client, lazily creating the same 30s-timeout default as New
// when no WithHTTPClient has been applied yet.
func (s *optionState) ensure() *http.Client {
	if s.client == nil {
		s.client = &http.Client{Timeout: 30 * time.Second}
	}
	return s.client
}

// Option configures a Client built via NewWithOptions. Options are applied in the order given,
// each mutating the in-progress HTTP client, so the last option to set a given field wins.
type Option func(*optionState)

// WithHTTPClient supplies a fully-configured *http.Client for the SDK to use. This gives the
// caller complete control over transport, timeout, redirects, and cookie handling. Applying it
// replaces the in-progress client outright — so a WithTransport / WithTimeout that comes AFTER
// it mutates this client, while one that came BEFORE it is discarded (last-wins). A nil client
// is ignored.
func WithHTTPClient(hc *http.Client) Option {
	return func(s *optionState) {
		if hc != nil {
			s.client = hc
		}
	}
}

// WithTransport sets the http.RoundTripper on the in-progress *http.Client (the current
// WithHTTPClient client, or the 30s-timeout default if none has been applied yet). Use this to
// plug in a proxy, custom CA / mTLS, or connection-pooling transport.
func WithTransport(rt http.RoundTripper) Option {
	return func(s *optionState) {
		s.ensure().Transport = rt
	}
}

// WithTimeout sets the Timeout on the in-progress *http.Client (the current WithHTTPClient
// client, or the 30s-timeout default if none has been applied yet).
func WithTimeout(d time.Duration) Option {
	return func(s *optionState) {
		s.ensure().Timeout = d
	}
}

// NewWithOptions builds a client like New, but accepts functional options for HTTP behavior.
// It is additive to New: with no options it behaves identically, defaulting to an
// &http.Client{Timeout: 30 * time.Second}.
//
// Options are applied in the order given, each mutating the in-progress *http.Client, so the
// last option to set a given field wins across all option types. For example
// WithTransport(rt), WithHTTPClient(hc) yields hc unchanged (the later WithHTTPClient discards
// the earlier transport), whereas WithHTTPClient(hc), WithTransport(rt) yields hc with rt.
func NewWithOptions(apiBaseURL, apiKey, privateKeyPKCS8B64 string, opts ...Option) (*Client, error) {
	var state optionState
	for _, opt := range opts {
		if opt != nil {
			opt(&state)
		}
	}

	httpClient := state.client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return New(apiBaseURL, apiKey, privateKeyPKCS8B64, httpClient)
}

// NewPublic builds a client for operations that do not decrypt data — listing banks
// (ListBanks), starting authorizations (StartAuthorization), and listing connections
// (GetConnections). It needs no encryption key; the decrypting methods (GetAccounts,
// GetTransactions, Sync, SyncAll) return an error on a client built this way.
func NewPublic(apiBaseURL, apiKey string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(apiBaseURL) == "" {
		return nil, fmt.Errorf("apiBaseUrl is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("apiKey is required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    strings.TrimRight(apiBaseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
	}, nil
}

// FromCredentials builds a client from a credentials-bundle JSON string or a path to a bundle file.
func FromCredentials(pathOrJSON string, httpClient *http.Client) (*Client, error) {
	raw := pathOrJSON
	if !strings.HasPrefix(strings.TrimSpace(pathOrJSON), "{") {
		// The caller deliberately supplies the path to their own credentials bundle.
		data, err := os.ReadFile(pathOrJSON) // #nosec G304 -- caller-provided credentials path by design
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

// errNoKey is returned by the decrypting methods when the client was built without an encryption
// key (see NewPublic).
func (c *Client) requireKey() error {
	if c.privateKey == nil {
		return fmt.Errorf("this client has no encryption key; build it from a credentials bundle to decrypt data")
	}
	return nil
}

// GetAccounts lists the user's accounts with all sensitive fields decrypted.
func (c *Client) GetAccounts() ([]Account, error) {
	if err := c.requireKey(); err != nil {
		return nil, err
	}
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
	if err := c.requireKey(); err != nil {
		return TransactionPage{}, err
	}
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
		conns = append(conns, Connection(w))
	}
	return conns, nil
}

// ListBanks lists the banks (ASPSPs) available for connection in a country (e.g. "DK").
func (c *Client) ListBanks(country string) ([]Bank, error) {
	path := "/api/aspsps"
	if country != "" {
		path += "?country=" + url.QueryEscape(country)
	}
	var wires []bankWire
	if err := c.getJSON(path, &wires); err != nil {
		return nil, err
	}
	banks := make([]Bank, 0, len(wires))
	for _, w := range wires {
		banks = append(banks, Bank(w))
	}
	return banks, nil
}

// StartAuthorization starts a bank-authorization (consent) flow and returns the consent URL the
// user must open in a browser to grant access. The bank's callback completes the flow server-side.
func (c *Client) StartAuthorization(req AuthorizationRequest) (string, error) {
	body := map[string]any{"country": req.Country, "aspspName": req.AspspName}
	if req.PsuType != "" {
		body["psuType"] = req.PsuType
	}
	var out authorizationURLWire
	if err := c.postJSON("/api/authorizations", body, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// Sync triggers an online sync of one account: decrypts that account's Enable Banking uid and posts
// it, so the service can fetch fresh data without ever holding the uid in plaintext.
func (c *Client) Sync(accountID string) (SyncResult, error) {
	if err := c.requireKey(); err != nil {
		return SyncResult{}, err
	}
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
	return SyncResult(result), nil
}

// SyncAll triggers an online sync of every account that has an active session.
func (c *Client) SyncAll() (SyncAllResult, error) {
	if err := c.requireKey(); err != nil {
		return SyncAllResult{}, err
	}
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
	return SyncAllResult(result), nil
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
	req.Header.Set("User-Agent", "open-banking-io/go/"+Version)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s failed: %w", req.Method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
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
