"""Diagnostics: a redacted, pasteable connectivity report and opt-in request logging."""

import json
import logging

from open_banking_io import OpenBankingClient

ACCOUNT_ID = "11111111-1111-4111-8111-111111111111"


def _client(base_url, credentials, **kw):
    return OpenBankingClient(
        api_base_url=base_url,
        api_key=credentials["apiKey"],
        private_key_pkcs8=credentials["encryptionKey"]["privateKey"],
        **kw,
    )


# -- The happy path ----------------------------------------------------------


def test_diagnose_against_a_live_server_passes_every_check(httpserver, credentials):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        diag = client.diagnose()

    assert diag.ok is True
    names = [c.name for c in diag.checks]
    assert names == ["base_url", "dns", "tcp_connect", "tls_handshake", "api_preflight"]
    # Plain http to loopback: the TLS stage is skipped, not failed.
    tls = next(c for c in diag.checks if c.name == "tls_handshake")
    assert tls.ok is True
    assert "skipped" in tls.detail.lower()
    preflight = next(c for c in diag.checks if c.name == "api_preflight")
    assert "200" in preflight.detail


def test_report_is_a_readable_string(httpserver, credentials):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        report = client.diagnose().report()

    assert "open-banking.io diagnostics" in report
    assert "base_url" in report
    assert "api_preflight" in report


# -- Redaction: the whole point of a pasteable report ------------------------


def test_report_never_contains_the_api_key_or_private_key(httpserver, credentials):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        diag = client.diagnose()
        report = diag.report()

    assert credentials["apiKey"] not in report
    assert credentials["encryptionKey"]["privateKey"] not in report
    assert credentials["apiKey"] not in json.dumps(diag.as_dict())
    assert credentials["encryptionKey"]["privateKey"] not in json.dumps(diag.as_dict())


def test_report_identifies_the_key_by_fingerprint_only(httpserver, credentials):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        diag = client.diagnose()

    fp = diag.environment["api_key_fingerprint"]
    assert len(fp) == 16
    assert fp not in credentials["apiKey"]


def test_proxy_env_vars_are_reported_by_name_with_values_redacted(
    httpserver, credentials, monkeypatch
):
    monkeypatch.setenv("HTTPS_PROXY", "http://user:hunter2@proxy.internal:3128")
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        diag = client.diagnose()
        report = diag.report()

    assert "HTTPS_PROXY" in report
    assert "hunter2" not in report
    assert "proxy.internal" not in report


# -- Failure paths: a diagnostic must never raise ----------------------------


def test_dns_failure_is_reported_not_raised(credentials):
    with _client("https://nonexistent.invalid", credentials) as client:
        diag = client.diagnose()

    assert diag.ok is False
    dns = next(c for c in diag.checks if c.name == "dns")
    assert dns.ok is False
    # Stages after the first failure are not attempted.
    assert next(c for c in diag.checks if c.name == "tcp_connect").skipped is True


def test_connection_refused_is_reported_not_raised(credentials):
    # Port 9 is the discard port; nothing listens on loopback there.
    with _client("http://127.0.0.1:9", credentials) as client:
        diag = client.diagnose()

    assert diag.ok is False
    assert next(c for c in diag.checks if c.name == "dns").ok is True
    assert next(c for c in diag.checks if c.name == "tcp_connect").ok is False


def test_an_unauthorized_preflight_is_reported_as_a_failed_check(httpserver, credentials):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json(
        {"error": "unauthorized"}, status=401
    )

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        diag = client.diagnose()

    preflight = next(c for c in diag.checks if c.name == "api_preflight")
    assert preflight.ok is False
    assert "401" in preflight.detail


def test_environment_reports_versions(httpserver, credentials):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        env = client.diagnose().environment

    for key in ("sdk_version", "python_version", "httpx_version", "openssl_version", "platform"):
        assert env[key]


# -- Opt-in request logging --------------------------------------------------


def test_requests_are_logged_without_headers_or_bodies(httpserver, credentials, caplog):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with (
        caplog.at_level(logging.DEBUG, logger="open_banking_io"),
        _client(httpserver.url_for("").rstrip("/"), credentials) as client,
    ):
        client.get_accounts()

    messages = [r.getMessage() for r in caplog.records if r.name == "open_banking_io"]
    assert any("GET" in m and "/api/accounts" in m and "200" in m for m in messages)
    joined = " ".join(messages)
    assert credentials["apiKey"] not in joined
    assert "X-Api-Key" not in joined


def test_nothing_is_logged_when_the_logger_is_not_enabled(httpserver, credentials, caplog):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with (
        caplog.at_level(logging.CRITICAL, logger="open_banking_io"),
        _client(httpserver.url_for("").rstrip("/"), credentials) as client,
    ):
        client.get_accounts()

    assert [r for r in caplog.records if r.name == "open_banking_io"] == []


def test_an_injected_http_client_is_not_instrumented(httpserver, credentials):
    """A caller-supplied client is used as-is, so we must not mutate its event hooks."""
    import httpx

    injected = httpx.Client()
    hooks_before = {k: list(v) for k, v in injected.event_hooks.items()}
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with _client(httpserver.url_for("").rstrip("/"), credentials, http_client=injected) as client:
        client.get_accounts()

    assert {k: list(v) for k, v in injected.event_hooks.items()} == hooks_before
