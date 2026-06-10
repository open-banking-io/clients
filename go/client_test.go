package openbanking

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

var transactionsPath = regexp.MustCompile(`^/api/accounts/[^/]+/transactions$`)
var accountSyncPath = regexp.MustCompile(`^/api/accounts/[^/]+/sync$`)

type mockAPI struct {
	server       *httptest.Server
	apiKey       string
	privateKey   string
	lastSyncBody map[string]any
}

func startMock(t *testing.T) *mockAPI {
	t.Helper()
	m := &mockAPI{
		apiKey:     readJSON(t, "credentials.json")["apiKey"].(string),
		privateKey: testPrivateKey(t),
	}

	send := func(w http.ResponseWriter, status int, body []byte) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != m.apiKey {
			send(w, http.StatusUnauthorized, []byte(`{"error":"unauthorized"}`))
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/accounts":
			send(w, http.StatusOK, readFixture(t, "api/accounts.json"))
		case r.Method == http.MethodGet && transactionsPath.MatchString(r.URL.Path):
			send(w, http.StatusOK, readFixture(t, "api/transactions.json"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/connections":
			send(w, http.StatusOK, readFixture(t, "api/connections.json"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/aspsps":
			send(w, http.StatusOK, []byte(`[{"name":"Lunar","country":"`+r.URL.Query().Get("country")+`","bic":"LUNADK22","beta":false,"psuTypes":["business"]}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/authorizations":
			m.captureBody(r)
			send(w, http.StatusOK, []byte(`{"url":"https://bank.example/consent?state=abc"}`))
		case r.Method == http.MethodPost && accountSyncPath.MatchString(r.URL.Path):
			m.captureBody(r)
			send(w, http.StatusOK, readFixture(t, "api/sync.json"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/sync":
			m.captureBody(r)
			send(w, http.StatusOK, readFixture(t, "api/sync-all.json"))
		default:
			send(w, http.StatusNotFound, []byte(`{"error":"not found"}`))
		}
	}))
	t.Cleanup(m.server.Close)
	return m
}

func (m *mockAPI) captureBody(r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	m.lastSyncBody = nil
	_ = json.Unmarshal(raw, &m.lastSyncBody)
}

func (m *mockAPI) client(t *testing.T) *Client {
	t.Helper()
	c, err := New(m.server.URL, m.apiKey, m.privateKey, nil)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func TestGetAccountsDecryptsFields(t *testing.T) {
	m := startMock(t)
	accounts, err := m.client(t).GetAccounts()
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("len(accounts) = %d, want 1", len(accounts))
	}
	a := accounts[0]
	if a.Iban != "DK6466952001724927" {
		t.Errorf("iban = %q", a.Iban)
	}
	if a.OwnerName != "Tatic ApS" {
		t.Errorf("ownerName = %q", a.OwnerName)
	}
	if a.DisplayName != "Drift" {
		t.Errorf("displayName = %q", a.DisplayName)
	}
	if len(a.Balances) != 3 {
		t.Fatalf("len(balances) = %d, want 3", len(a.Balances))
	}
	byType := map[string]string{}
	for _, b := range a.Balances {
		byType[b.Type] = b.Amount
	}
	if byType["ITBD"] != "828.13" {
		t.Errorf("ITBD = %q, want 828.13", byType["ITBD"])
	}
	if byType["ITAV"] != "633.90" {
		t.Errorf("ITAV = %q, want 633.90", byType["ITAV"])
	}
}

func TestGetTransactionsDecryptsAmountAndCreditor(t *testing.T) {
	m := startMock(t)
	limit := 50
	page, err := m.client(t).GetTransactions("11111111-1111-4111-8111-111111111111", TransactionQuery{Limit: &limit})
	if err != nil {
		t.Fatalf("GetTransactions: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("total = %d, want 1", page.Total)
	}
	tx := page.Items[0]
	if tx.Amount != "194.23" {
		t.Errorf("amount = %q, want 194.23", tx.Amount)
	}
	if tx.CreditorName != "One.com" {
		t.Errorf("creditorName = %q, want One.com", tx.CreditorName)
	}
}

func TestGetConnections(t *testing.T) {
	m := startMock(t)
	conns, err := m.client(t).GetConnections()
	if err != nil {
		t.Fatalf("GetConnections: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("len(conns) = %d, want 1", len(conns))
	}
	if conns[0].AspspName != "Lunar" {
		t.Errorf("aspspName = %q", conns[0].AspspName)
	}
	if conns[0].Status != "Active" {
		t.Errorf("status = %q", conns[0].Status)
	}
}

func TestSyncPostsDecryptedUid(t *testing.T) {
	m := startMock(t)
	result, err := m.client(t).Sync("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.TotalFetched != 1 {
		t.Errorf("totalFetched = %d, want 1", result.TotalFetched)
	}
	if m.lastSyncBody["uid"] != "c5d93aa7-5e23-4da0-ba88-42b9a584492c" {
		t.Errorf("posted uid = %v", m.lastSyncBody["uid"])
	}
}

func TestSyncAllPostsItemsWithDecryptedUid(t *testing.T) {
	m := startMock(t)
	result, err := m.client(t).SyncAll()
	if err != nil {
		t.Fatalf("SyncAll: %v", err)
	}
	if result.Accounts != 1 {
		t.Errorf("accounts = %d, want 1", result.Accounts)
	}
	items, ok := m.lastSyncBody["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %v", m.lastSyncBody["items"])
	}
	item := items[0].(map[string]any)
	if item["accountId"] != "11111111-1111-4111-8111-111111111111" {
		t.Errorf("accountId = %v", item["accountId"])
	}
	if item["uid"] != "c5d93aa7-5e23-4da0-ba88-42b9a584492c" {
		t.Errorf("uid = %v", item["uid"])
	}
}

func TestListBanks(t *testing.T) {
	m := startMock(t)
	banks, err := m.client(t).ListBanks("DK")
	if err != nil {
		t.Fatalf("ListBanks: %v", err)
	}
	if len(banks) != 1 {
		t.Fatalf("len(banks) = %d, want 1", len(banks))
	}
	if banks[0].Name != "Lunar" || banks[0].Country != "DK" || banks[0].Bic != "LUNADK22" {
		t.Errorf("unexpected bank: %+v", banks[0])
	}
	if len(banks[0].PsuTypes) != 1 || banks[0].PsuTypes[0] != "business" {
		t.Errorf("psuTypes = %v", banks[0].PsuTypes)
	}
}

func TestStartAuthorizationReturnsConsentURLAndPostsRequest(t *testing.T) {
	m := startMock(t)
	url, err := m.client(t).StartAuthorization(AuthorizationRequest{
		Country: "DK", AspspName: "Lunar", PsuType: "business",
	})
	if err != nil {
		t.Fatalf("StartAuthorization: %v", err)
	}
	if url != "https://bank.example/consent?state=abc" {
		t.Errorf("url = %q", url)
	}
	if m.lastSyncBody["aspspName"] != "Lunar" || m.lastSyncBody["country"] != "DK" ||
		m.lastSyncBody["psuType"] != "business" {
		t.Errorf("posted body = %v", m.lastSyncBody)
	}
}

func TestPublicClientAllowsBanksButNotDecryption(t *testing.T) {
	m := startMock(t)
	c, err := NewPublic(m.server.URL, m.apiKey, nil)
	if err != nil {
		t.Fatalf("NewPublic: %v", err)
	}

	// Non-decrypting calls work without an encryption key.
	if _, err := c.ListBanks("DK"); err != nil {
		t.Errorf("ListBanks on public client: %v", err)
	}
	if _, err := c.GetConnections(); err != nil {
		t.Errorf("GetConnections on public client: %v", err)
	}

	// Decrypting calls fail cleanly rather than panicking.
	if _, err := c.GetAccounts(); err == nil {
		t.Error("expected GetAccounts to error on a key-less client")
	}
	if _, err := c.GetTransactions("11111111-1111-4111-8111-111111111111", TransactionQuery{}); err == nil {
		t.Error("expected GetTransactions to error on a key-less client")
	}
	if _, err := c.Sync("11111111-1111-4111-8111-111111111111"); err == nil {
		t.Error("expected Sync to error on a key-less client")
	}
	if _, err := c.SyncAll(); err == nil {
		t.Error("expected SyncAll to error on a key-less client")
	}
}

func TestWrongAPIKeyErrors(t *testing.T) {
	m := startMock(t)
	c, err := New(m.server.URL, "wrong-key", m.privateKey, nil)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := c.GetAccounts(); err == nil {
		t.Error("expected error with wrong API key")
	}
}

func TestWrongPrivateKeyErrorsOnDecryption(t *testing.T) {
	m := startMock(t)
	const badKey = "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgdB9vjCwdrF1FckSNDHrw8M7PNNkpUB/tAK/EgAbZWvihRANCAASgg8XOKlU9VeYCee9+tQKtDSkFze10CRTA4b2gGKDlHIFw+QTf1AkjAjLfWqCJ4BctUqQtAYs0v0Y90Bw1wsBF"
	c, err := New(m.server.URL, m.apiKey, badKey, nil)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := c.GetAccounts(); err == nil {
		t.Error("expected decryption error with wrong private key")
	}
}
