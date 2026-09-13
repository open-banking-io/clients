package openbanking

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Sync failure reasons. Branch on these, never on the HTTP status; a reason added later arrives as
// its raw string.
const (
	ReasonReconnectNeeded    = "reconnect_needed"
	ReasonConsentWithdrawn   = "consent_withdrawn"
	ReasonUidOutdated        = "uid_outdated"
	ReasonPsuPresentRequired = "psu_present_required"
	ReasonRateLimited        = "rate_limited"
	ReasonBankError          = "bank_error"
	ReasonTransient          = "transient"
	ReasonPartnerAppInactive = "partner_app_inactive"
)

// SyncError is a sync the service refused. Reason is the stable code to branch on.
type SyncError struct {
	Status            int
	Reason            string
	BankErrorCode     string
	RetryAfterSeconds int
	message           string
}

func (e *SyncError) Error() string { return e.message }

// SyncFailure is one account a bulk sync did not refresh, and why.
type SyncFailure struct {
	AccountID     string
	Reason        string
	BankErrorCode string
}

// PsuHeaders is the account holder's own request, forwarded while they are on your page: some banks
// only share data with the person present. Never send it from a background job.
type PsuHeaders struct {
	IPAddress      string
	UserAgent      string
	Referer        string
	Accept         string
	AcceptLanguage string
	AcceptCharset  string
	AcceptEncoding string
}

// SyncOptions carries optional request details for SyncWithOptions and SyncAllWithOptions.
type SyncOptions struct {
	Psu *PsuHeaders
}

func (o SyncOptions) headers() map[string]string {
	if o.Psu == nil {
		return nil
	}
	h := map[string]string{
		"X-Psu-Ip-Address": o.Psu.IPAddress,
		"X-Psu-User-Agent": o.Psu.UserAgent,
	}
	for name, value := range map[string]string{
		"X-Psu-Referer":         o.Psu.Referer,
		"X-Psu-Accept":          o.Psu.Accept,
		"X-Psu-Accept-Language": o.Psu.AcceptLanguage,
		"X-Psu-Accept-Charset":  o.Psu.AcceptCharset,
		"X-Psu-Accept-Encoding": o.Psu.AcceptEncoding,
	} {
		if value != "" {
			h[name] = value
		}
	}
	return h
}

func isUidOutdated(err error) bool {
	var se *SyncError
	return errors.As(err, &se) && se.Reason == ReasonUidOutdated
}

func syncErrorFrom(method, path string, resp *http.Response, body []byte) *SyncError {
	var problem struct {
		Reason        string `json:"reason"`
		BankErrorCode string `json:"bankErrorCode"`
	}
	_ = json.Unmarshal(body, &problem)
	retryAfter, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
	detail := resp.Status
	if problem.Reason != "" {
		detail = problem.Reason
	}
	return &SyncError{
		Status:            resp.StatusCode,
		Reason:            problem.Reason,
		BankErrorCode:     problem.BankErrorCode,
		RetryAfterSeconds: retryAfter,
		message:           fmt.Sprintf("%s %s failed: %d %s", method, path, resp.StatusCode, detail),
	}
}

func connectionFrom(w connectionWire, now time.Time) Connection {
	live := w.Status == "Active"
	if until, err := time.Parse(time.RFC3339, w.ValidUntil); err == nil {
		live = live && until.After(now)
	}
	if w.IsLive != nil {
		live = *w.IsLive
	}
	ids := w.AccountIDs
	if ids == nil {
		ids = []string{}
	}
	return Connection{
		SessionID:    w.SessionID,
		AspspName:    w.AspspName,
		AspspCountry: w.AspspCountry,
		ValidUntil:   w.ValidUntil,
		Status:       w.Status,
		AccountCount: w.AccountCount,
		LastSyncedAt: w.LastSyncedAt,
		PsuType:      w.PsuType,
		IsLive:       live,
		AccountIDs:   ids,
	}
}
