package openbanking

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeAPI is a configurable mock whose account/transaction responses each test can override to
// exercise error and edge-case branches (bad ciphertext, missing sessions, empty pages, ...).
type fakeAPI struct {
	server       *httptest.Server
	apiKey       string
	privateKey   string
	accounts     string // body for GET /api/accounts
	transactions string // body for GET /api/accounts/{id}/transactions
	postStatus   int    // when non-zero, POST routes reply with this status (to fail postJSON)
	lastQuery    string
	lastBody     map[string]any
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{
		apiKey:       readJSON(t, "credentials.json")["apiKey"].(string),
		privateKey:   testPrivateKey(t),
		accounts:     string(readFixture(t, "api/accounts.json")),
		transactions: string(readFixture(t, "api/transactions.json")),
	}
	capture := func(r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.lastBody = nil
		_ = json.Unmarshal(raw, &f.lastBody)
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Api-Key") != f.apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/accounts":
			_, _ = io.WriteString(w, f.accounts)
		case r.Method == http.MethodGet && transactionsPath.MatchString(r.URL.Path):
			_, _ = io.WriteString(w, f.transactions)
		case r.Method == http.MethodPost && accountSyncPath.MatchString(r.URL.Path):
			capture(r)
			if f.postStatus != 0 {
				w.WriteHeader(f.postStatus)
				return
			}
			_, _ = io.WriteString(w, `{"newTransactions":0,"totalFetched":0}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/sync":
			capture(r)
			if f.postStatus != 0 {
				w.WriteHeader(f.postStatus)
				return
			}
			_, _ = io.WriteString(w, `{"accounts":0,"newTransactions":0}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeAPI) client(t *testing.T) *Client {
	t.Helper()
	c, err := New(f.server.URL, f.apiKey, f.privateKey, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// An account whose ciphertext fields are all empty: decrypts to zero values, never an error.
const acctNoCiphertext = `[{"id":"acc-1","aspspName":"Lunar","aspspCountry":"DK","currency":"DKK",` +
	`"accountType":"CACC","bic":"LUNADK22","needsReconnect":true,"balances":null,` +
	`"enc":"","displayNameEnc":"","uidEnc":""}]`

// ---- HTTP transport / status / decode failures (do, getJSON) ----

func serve(t *testing.T, status int, body string) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func clientAt(t *testing.T, url string) *Client {
	t.Helper()
	c, err := New(url, readJSON(t, "credentials.json")["apiKey"].(string), testPrivateKey(t), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNon2xxStatusErrors(t *testing.T) {
	c := clientAt(t, serve(t, http.StatusInternalServerError, `{"error":"boom"}`))
	if _, err := c.GetConnections(); err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestUnparseableBodyErrors(t *testing.T) {
	c := clientAt(t, serve(t, http.StatusOK, "this is not json"))
	if _, err := c.GetConnections(); err == nil {
		t.Error("expected error for unparseable body")
	}
}

func TestTransportFailureErrors(t *testing.T) {
	// Start a server only to obtain a guaranteed-dead URL, then close it: the connection is refused.
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := s.URL
	s.Close()
	c := clientAt(t, url)
	if _, err := c.GetConnections(); err == nil {
		t.Error("expected transport error against a closed server")
	}
}

// ---- GetTransactions query building and edge cases ----

func TestGetTransactionsEncodesAllQueryParams(t *testing.T) {
	f := newFakeAPI(t)
	f.transactions = `{"items":[],"total":0}`
	limit, offset := 50, 10
	_, err := f.client(t).GetTransactions("acc-1", TransactionQuery{
		From: "2026-01-01", To: "2026-02-01", Limit: &limit, Offset: &offset,
	})
	if err != nil {
		t.Fatalf("GetTransactions: %v", err)
	}
	for _, want := range []string{"from=2026-01-01", "to=2026-02-01", "limit=50", "offset=10"} {
		if !strings.Contains(f.lastQuery, want) {
			t.Errorf("query %q missing %q", f.lastQuery, want)
		}
	}
}

func TestGetTransactionsEmptyPage(t *testing.T) {
	f := newFakeAPI(t)
	f.transactions = `{"total":0}` // no items field
	page, err := f.client(t).GetTransactions("acc-1", TransactionQuery{})
	if err != nil {
		t.Fatalf("GetTransactions: %v", err)
	}
	if len(page.Items) != 0 || page.Total != 0 {
		t.Errorf("page = %+v, want empty", page)
	}
}

func TestGetTransactionsDecryptErrorSurfaces(t *testing.T) {
	f := newFakeAPI(t)
	f.transactions = `{"items":[{"id":"tx-1","enc":"not!base64"}],"total":1}`
	if _, err := f.client(t).GetTransactions("acc-1", TransactionQuery{}); err == nil {
		t.Error("expected error from undecryptable transaction ciphertext")
	}
}

// ---- GetAccounts decrypt edge cases (mapAccount) ----

func TestGetAccountsWithEmptyCiphertextDecryptsToZeroValues(t *testing.T) {
	f := newFakeAPI(t)
	f.accounts = acctNoCiphertext
	accounts, err := f.client(t).GetAccounts()
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("len = %d, want 1", len(accounts))
	}
	a := accounts[0]
	if a.Iban != "" || a.OwnerName != "" || a.DisplayName != "" {
		t.Errorf("expected empty decrypted fields, got %+v", a)
	}
	if len(a.Balances) != 0 {
		t.Errorf("len(balances) = %d, want 0", len(a.Balances))
	}
}

func TestGetAccountsDecryptErrorSurfaces(t *testing.T) {
	f := newFakeAPI(t)
	f.accounts = `[{"id":"acc-1","balances":null,"enc":"not!base64","displayNameEnc":"","uidEnc":""}]`
	if _, err := f.client(t).GetAccounts(); err == nil {
		t.Error("expected error from undecryptable account ciphertext")
	}
}

func TestGetAccountsDisplayNameDecryptErrorSurfaces(t *testing.T) {
	f := newFakeAPI(t)
	// enc is empty (decrypts to nothing), but the display-name ciphertext is undecryptable.
	f.accounts = `[{"id":"acc-1","balances":null,"enc":"","displayNameEnc":"not!base64","uidEnc":""}]`
	if _, err := f.client(t).GetAccounts(); err == nil {
		t.Error("expected error from undecryptable display-name ciphertext")
	}
}

func TestGetAccountsBalanceDecryptErrorSurfaces(t *testing.T) {
	f := newFakeAPI(t)
	f.accounts = `[{"id":"acc-1","enc":"","displayNameEnc":"","uidEnc":"",` +
		`"balances":[{"type":"ITBD","currency":"DKK","enc":"not!base64"}]}]`
	if _, err := f.client(t).GetAccounts(); err == nil {
		t.Error("expected error from undecryptable balance ciphertext")
	}
}

// ---- Sync / SyncAll edge cases ----

func TestSyncUnknownAccountErrors(t *testing.T) {
	f := newFakeAPI(t)
	f.accounts = `[]`
	if _, err := f.client(t).Sync("missing"); err == nil {
		t.Error("expected error for unknown account")
	}
}

func TestSyncAccountWithoutSessionErrors(t *testing.T) {
	f := newFakeAPI(t)
	f.accounts = acctNoCiphertext // empty uidEnc => no active session
	_, err := f.client(t).Sync("acc-1")
	if err == nil || !strings.Contains(err.Error(), "no active session") {
		t.Errorf("err = %v, want 'no active session'", err)
	}
}

func TestSyncDecryptUidErrorSurfaces(t *testing.T) {
	f := newFakeAPI(t)
	f.accounts = `[{"id":"acc-1","balances":null,"enc":"","displayNameEnc":"","uidEnc":"not!base64"}]`
	if _, err := f.client(t).Sync("acc-1"); err == nil {
		t.Error("expected error from undecryptable uid ciphertext")
	}
}

func TestSyncListAccountsErrorSurfaces(t *testing.T) {
	c := clientAt(t, serve(t, http.StatusInternalServerError, `{}`))
	if _, err := c.Sync("acc-1"); err == nil {
		t.Error("expected error when listing accounts fails")
	}
	if _, err := c.SyncAll(); err == nil {
		t.Error("expected error when listing accounts fails")
	}
}

func TestSyncAllSkipsAccountsWithoutSession(t *testing.T) {
	f := newFakeAPI(t)
	f.accounts = acctNoCiphertext // no active session => skipped
	if _, err := f.client(t).SyncAll(); err != nil {
		t.Fatalf("SyncAll: %v", err)
	}
	items, ok := f.lastBody["items"].([]any)
	if !ok || len(items) != 0 {
		t.Errorf("posted items = %v, want empty list", f.lastBody["items"])
	}
}

// ---- ListBanks / StartAuthorization branch coverage ----

func TestListBanksWithoutCountry(t *testing.T) {
	m := startMock(t)
	banks, err := m.client(t).ListBanks("") // no country => path without query string
	if err != nil {
		t.Fatalf("ListBanks: %v", err)
	}
	if len(banks) != 1 {
		t.Fatalf("len(banks) = %d, want 1", len(banks))
	}
}

func TestListBanksHTTPErrorSurfaces(t *testing.T) {
	c := clientAt(t, serve(t, http.StatusInternalServerError, `{}`))
	if _, err := c.ListBanks("DK"); err == nil {
		t.Error("expected error from ListBanks on 500")
	}
}

func TestGetTransactionsHTTPErrorSurfaces(t *testing.T) {
	c := clientAt(t, serve(t, http.StatusInternalServerError, `{}`))
	if _, err := c.GetTransactions("acc-1", TransactionQuery{}); err == nil {
		t.Error("expected error from GetTransactions on 500")
	}
}

func TestStartAuthorizationHTTPErrorSurfaces(t *testing.T) {
	c := clientAt(t, serve(t, http.StatusInternalServerError, `{}`))
	if _, err := c.StartAuthorization(AuthorizationRequest{Country: "DK", AspspName: "Lunar"}); err == nil {
		t.Error("expected error from StartAuthorization on 500")
	}
}

func TestSyncPostErrorSurfaces(t *testing.T) {
	f := newFakeAPI(t)
	f.postStatus = http.StatusInternalServerError // account is found and has a session; the POST fails
	if _, err := f.client(t).Sync("11111111-1111-4111-8111-111111111111"); err == nil {
		t.Error("expected error when the sync POST fails")
	}
}

func TestSyncAllPostErrorSurfaces(t *testing.T) {
	f := newFakeAPI(t)
	f.postStatus = http.StatusInternalServerError
	if _, err := f.client(t).SyncAll(); err == nil {
		t.Error("expected error when the sync-all POST fails")
	}
}

func TestStartAuthorizationWithoutPsuType(t *testing.T) {
	m := startMock(t)
	url, err := m.client(t).StartAuthorization(AuthorizationRequest{Country: "DK", AspspName: "Lunar"})
	if err != nil {
		t.Fatalf("StartAuthorization: %v", err)
	}
	if url == "" {
		t.Error("expected a consent url")
	}
	if _, present := m.lastSyncBody["psuType"]; present {
		t.Error("psuType should be omitted when empty")
	}
}
