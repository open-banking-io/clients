package app

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// CACertEnv names an optional path to a PEM file holding one or more root CA
// certificates. When set, the CLI's HTTP client trusts those roots in addition
// to the system trust store — useful behind a TLS-inspecting proxy or against a
// staging environment fronted by a private CA. Empty/whitespace is ignored.
const CACertEnv = "OPENBANKING_CA_CERT"

// BuildHTTPClient returns a custom *http.Client when the user has opted in — via
// a positive timeout or a custom root CA path — and (nil, nil) otherwise, which
// leaves App.HTTPClient nil so the SDK keeps its default client (unchanged
// behavior). The returned client carries the request timeout and, when a CA path
// is given, a transport whose TLS config trusts the extra root(s).
//
// Proxying needs no wiring here: the default transport and the custom one both
// use http.ProxyFromEnvironment, so HTTPS_PROXY / HTTP_PROXY / NO_PROXY are
// honored automatically.
func BuildHTTPClient(timeout time.Duration, caCertPath string) (*http.Client, error) {
	caCertPath = strings.TrimSpace(caCertPath)
	if timeout <= 0 && caCertPath == "" {
		return nil, nil
	}

	client := &http.Client{Timeout: timeout}
	if caCertPath == "" {
		// Timeout only: leave Transport nil so net/http uses its default
		// transport (which already honors the proxy env vars).
		return client, nil
	}

	pem, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", CACertEnv, err)
	}
	// Extend the system roots rather than replacing them, so normal HTTPS still
	// works alongside the extra CA. A fresh pool is the fallback when the system
	// store is unavailable (e.g. some minimal containers).
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("%s: no certificates found in %s", CACertEnv, caCertPath)
	}
	client.Transport = &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}
	return client, nil
}
