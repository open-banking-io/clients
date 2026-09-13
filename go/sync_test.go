package openbanking

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	testAccount = "11111111-1111-4111-8111-111111111111"
	testSibling = "66666666-6666-4666-8666-666666666666"
	testUid     = "c5d93aa7-5e23-4da0-ba88-42b9a584492c"
)

type recorded struct {
	Method  string
	Path    string
	Header  http.Header
	Body    map[string]any
	ByPath  int
	Written bool
}

type route func(r recorded, w http.ResponseWriter) bool

func twoAccountsFixture(t *testing.T) []byte {
	t.Helper()
	var accounts []map[string]any
	if err := json.Unmarshal(readFixture(t, "api/accounts.json"), &accounts); err != nil {
		t.Fatal(err)
	}
	sibling := map[string]any{}
	for k, v := range accounts[0] {
		sibling[k] = v
	}
	sibling["id"] = testSibling
	out, _ := json.Marshal(append(accounts, sibling))
	return out
}

func stubAPI(t *testing.T, accounts []byte, handle route) (*Client, *[]recorded) {
	t.Helper()
	if accounts == nil {
		accounts = readFixture(t, "api/accounts.json")
	}
	calls := []recorded{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec := recorded{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone()}
		_ = json.Unmarshal(raw, &rec.Body)
		for _, c := range calls {
			if c.Path == rec.Path {
				rec.ByPath++
			}
		}
		calls = append(calls, rec)
		if r.Method == http.MethodGet && r.URL.Path == "/api/accounts" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(accounts)
			return
		}
		if !handle(rec, w) {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	c, err := New(server.URL, "ebk_test", testPrivateKey(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, &calls
}

func reply(w http.ResponseWriter, status int, body string) bool {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
	return true
}

func TestConnections_CarryIsLiveAndAccountIDs(t *testing.T) {
	c, _ := stubAPI(t, nil, func(r recorded, w http.ResponseWriter) bool {
		if r.Path != "/api/connections" {
			return false
		}
		_, _ = w.Write(readFixture(t, "api/connections.json"))
		return true
	})

	conns, err := c.GetConnections()
	if err != nil {
		t.Fatal(err)
	}
	if conns[0].IsLive {
		t.Errorf("IsLive = true, want the service's false")
	}
	if !reflect.DeepEqual(conns[0].AccountIDs, []string{testAccount}) {
		t.Errorf("AccountIDs = %v", conns[0].AccountIDs)
	}
}

func TestConnections_TheServicesIsLiveWinsOverTheLocalClock(t *testing.T) {
	c, _ := stubAPI(t, nil, func(r recorded, w http.ResponseWriter) bool {
		if r.Path != "/api/connections" {
			return false
		}
		return reply(w, 200, `[{"sessionId":"a","status":"Active","validUntil":"2020-01-01T00:00:00Z","isLive":true}]`)
	})

	conns, err := c.GetConnections()
	if err != nil {
		t.Fatal(err)
	}
	if !conns[0].IsLive {
		t.Errorf("IsLive = false, want the service's true")
	}
}

func TestConnections_WithoutIsLive_DecideFromStatusAndValidUntil(t *testing.T) {
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	c, _ := stubAPI(t, nil, func(r recorded, w http.ResponseWriter) bool {
		if r.Path != "/api/connections" {
			return false
		}
		return reply(w, 200, `[
			{"sessionId":"a","status":"Active","validUntil":"`+future+`"},
			{"sessionId":"b","status":"Active","validUntil":"2020-01-01T00:00:00Z"},
			{"sessionId":"c","status":"Revoked","validUntil":"`+future+`"}]`)
	})

	conns, err := c.GetConnections()
	if err != nil {
		t.Fatal(err)
	}
	got := []bool{conns[0].IsLive, conns[1].IsLive, conns[2].IsLive}
	if !reflect.DeepEqual(got, []bool{true, false, false}) {
		t.Errorf("IsLive = %v, want [true false false]", got)
	}
	if conns[0].AccountIDs == nil || len(conns[0].AccountIDs) != 0 {
		t.Errorf("AccountIDs = %#v, want empty", conns[0].AccountIDs)
	}
}

func TestSyncAll_ReturnsEveryFailure(t *testing.T) {
	c, _ := stubAPI(t, nil, func(r recorded, w http.ResponseWriter) bool {
		if r.Path != "/api/sync" {
			return false
		}
		_, _ = w.Write(readFixture(t, "api/sync-all-failures.json"))
		return true
	})

	result, err := c.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	want := []SyncFailure{
		{AccountID: "33333333-3333-4333-8333-333333333333", Reason: ReasonReconnectNeeded, BankErrorCode: "ASPSP_ACCOUNT_NOT_ACCESSIBLE"},
		{AccountID: "44444444-4444-4444-8444-444444444444", Reason: ReasonPsuPresentRequired, BankErrorCode: "PSU_HEADER_NOT_PROVIDED"},
		{AccountID: "55555555-5555-4555-8555-555555555555", Reason: ReasonRateLimited},
	}
	if result.Accounts != 1 || result.NewTransactions != 4 || !reflect.DeepEqual(result.Failures, want) {
		t.Errorf("result = %+v", result)
	}
}

func TestSync_RefusalIsASyncError(t *testing.T) {
	c, _ := stubAPI(t, nil, func(r recorded, w http.ResponseWriter) bool {
		if r.Method != http.MethodPost {
			return false
		}
		w.Header().Set("Retry-After", "3600")
		return reply(w, 429, `{"status":429,"reason":"rate_limited","bankErrorCode":"ASPSP_RATE_LIMIT_EXCEEDED"}`)
	})

	_, err := c.Sync(testAccount)

	var se *SyncError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want *SyncError", err)
	}
	if se.Status != 429 || se.Reason != ReasonRateLimited || se.BankErrorCode != "ASPSP_RATE_LIMIT_EXCEEDED" || se.RetryAfterSeconds != 3600 {
		t.Errorf("SyncError = %+v", se)
	}
}

func TestSync_RetriesUidOutdatedExactlyOnce(t *testing.T) {
	c, calls := stubAPI(t, nil, func(r recorded, w http.ResponseWriter) bool {
		if r.Method != http.MethodPost {
			return false
		}
		if r.ByPath == 0 {
			return reply(w, 409, `{"reason":"uid_outdated"}`)
		}
		return reply(w, 200, `{"newTransactions":2,"totalFetched":5}`)
	})

	result, err := c.Sync(testAccount)
	if err != nil {
		t.Fatal(err)
	}
	if result.NewTransactions != 2 || result.TotalFetched != 5 {
		t.Errorf("result = %+v", result)
	}
	var sequence []string
	for _, call := range *calls {
		sequence = append(sequence, call.Method+" "+call.Path)
	}
	want := []string{"GET /api/accounts", "POST /api/accounts/" + testAccount + "/sync", "GET /api/accounts", "POST /api/accounts/" + testAccount + "/sync"}
	if !reflect.DeepEqual(sequence, want) {
		t.Errorf("calls = %v", sequence)
	}
}

func TestSync_GivesUpAfterASecondUidOutdated(t *testing.T) {
	c, calls := stubAPI(t, nil, func(r recorded, w http.ResponseWriter) bool {
		if r.Method != http.MethodPost {
			return false
		}
		return reply(w, 409, `{"reason":"uid_outdated"}`)
	})

	_, err := c.Sync(testAccount)

	if !isUidOutdated(err) {
		t.Fatalf("err = %v, want uid_outdated", err)
	}
	posts := 0
	for _, call := range *calls {
		if call.Method == http.MethodPost {
			posts++
		}
	}
	if posts != 2 {
		t.Errorf("posts = %d, want 2", posts)
	}
}

func TestSyncAll_RetriesOnlyOutdatedAccounts_AndMergesCounts(t *testing.T) {
	c, calls := stubAPI(t, twoAccountsFixture(t), func(r recorded, w http.ResponseWriter) bool {
		if r.Path != "/api/sync" {
			return false
		}
		if r.ByPath == 0 {
			return reply(w, 200, `{"accounts":1,"newTransactions":3,"failures":[
				{"accountId":"`+testAccount+`","reason":"uid_outdated"},
				{"accountId":"33333333-3333-4333-8333-333333333333","reason":"reconnect_needed"}]}`)
		}
		return reply(w, 200, `{"accounts":1,"newTransactions":7,"failures":[]}`)
	})

	result, err := c.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	var posts []recorded
	for _, call := range *calls {
		if call.Path == "/api/sync" {
			posts = append(posts, call)
		}
	}
	if len(posts) != 2 {
		t.Fatalf("posts = %d, want 2", len(posts))
	}
	if n := len(posts[0].Body["items"].([]any)); n != 2 {
		t.Errorf("first post items = %d, want 2", n)
	}
	retry := posts[1].Body["items"].([]any)
	if len(retry) != 1 || retry[0].(map[string]any)["accountId"] != testAccount || retry[0].(map[string]any)["uid"] != testUid {
		t.Errorf("retry items = %v", retry)
	}
	want := SyncAllResult{
		Accounts:        2,
		NewTransactions: 10,
		Failures:        []SyncFailure{{AccountID: "33333333-3333-4333-8333-333333333333", Reason: ReasonReconnectNeeded}},
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("result = %+v", result)
	}
}

func TestSyncAll_ReportsUidOutdated_WhenTheRetryIsRefusedAgain(t *testing.T) {
	c, _ := stubAPI(t, nil, func(r recorded, w http.ResponseWriter) bool {
		if r.Path != "/api/sync" {
			return false
		}
		return reply(w, 200, `{"accounts":0,"newTransactions":0,"failures":[{"accountId":"`+testAccount+`","reason":"uid_outdated"}]}`)
	})

	result, err := c.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Failures, []SyncFailure{{AccountID: testAccount, Reason: ReasonUidOutdated}}) {
		t.Errorf("failures = %+v", result.Failures)
	}
}

func TestSyncWithOptions_ForwardsThePresentUsersHeaders(t *testing.T) {
	c, calls := stubAPI(t, nil, func(r recorded, w http.ResponseWriter) bool {
		if r.Path == "/api/sync" {
			return reply(w, 200, `{"accounts":1,"newTransactions":0}`)
		}
		if r.Method == http.MethodPost {
			return reply(w, 200, `{"newTransactions":0,"totalFetched":0}`)
		}
		return false
	})
	opts := SyncOptions{Psu: &PsuHeaders{IPAddress: "203.0.113.7", UserAgent: "Mozilla/5.0", AcceptLanguage: "da-DK"}}

	if _, err := c.SyncWithOptions(testAccount, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SyncAllWithOptions(opts); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SyncAll(); err != nil {
		t.Fatal(err)
	}

	var posts []recorded
	for _, call := range *calls {
		if call.Method == http.MethodPost {
			posts = append(posts, call)
		}
	}
	for _, p := range posts[:2] {
		if p.Header.Get("X-Psu-Ip-Address") != "203.0.113.7" || p.Header.Get("X-Psu-User-Agent") != "Mozilla/5.0" ||
			p.Header.Get("X-Psu-Accept-Language") != "da-DK" || p.Header.Get("X-Api-Key") != "ebk_test" {
			t.Errorf("%s headers = %v", p.Path, p.Header)
		}
		if _, sent := p.Header["X-Psu-Referer"]; sent {
			t.Errorf("%s sent an empty X-Psu-Referer", p.Path)
		}
	}
	for name := range posts[2].Header {
		if strings.HasPrefix(name, "X-Psu-") {
			t.Errorf("plain SyncAll sent %s", name)
		}
	}
}
