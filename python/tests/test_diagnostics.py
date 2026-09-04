"""Diagnostics: a redacted, pasteable connectivity report and opt-in request logging."""

import json
import logging

import httpx
import pytest

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
    # Plain http to loopback: the TLS stage is skipped, not failed -- and says so in the
    # `skipped` field, not just in prose.
    tls = next(c for c in diag.checks if c.name == "tls_handshake")
    assert tls.ok is True
    assert tls.skipped is True
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


def test_the_api_key_is_described_never_shown(httpserver, credentials):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        diag = client.diagnose()

    assert diag.environment["api_key"] == f"set ({len(credentials['apiKey'])} chars)"


def test_credentials_in_the_base_url_are_stripped_from_the_report(httpserver, credentials):
    """A base url may carry basic-auth userinfo; it must not reach a pasteable report."""
    host = httpserver.url_for("").rstrip("/").split("://", 1)[1]
    with _client(f"http://alice:hunter2@{host}", credentials) as client:
        diag = client.diagnose()
        report = diag.report()

    assert "hunter2" not in report
    assert "alice" not in report
    assert "hunter2" not in json.dumps(diag.as_dict())


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
    # A probe that depends on a failed one is skipped ...
    assert next(c for c in diag.checks if c.name == "tcp_connect").skipped is True
    # ... but the preflight always runs, since it is the authoritative check.
    preflight = next(c for c in diag.checks if c.name == "api_preflight")
    assert preflight.skipped is False
    assert preflight.ok is False


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


def test_preflight_runs_even_when_the_direct_probes_fail(httpserver, credentials):
    """A proxy or custom TLS setup can make the direct probes fail on a working connection,
    so the verdict comes from the request the SDK actually makes."""
    import httpx

    # Stands in for a proxy: the host never resolves directly, but the configured
    # transport delivers the request anyway.
    transport = httpx.MockTransport(lambda request: httpx.Response(200, json=[]))
    proxied = httpx.Client(transport=transport)

    with _client("https://nonexistent.invalid", credentials, http_client=proxied) as client:
        diag = client.diagnose()

    assert next(c for c in diag.checks if c.name == "dns").ok is False
    preflight = next(c for c in diag.checks if c.name == "api_preflight")
    assert preflight.ok is True
    assert "proxy" in preflight.detail
    assert diag.ok is True


# -- Never raises, and always returns all five checks ------------------------

CHECK_NAMES = ["base_url", "dns", "tcp_connect", "tls_handshake", "api_preflight"]


@pytest.mark.parametrize(
    "hostile",
    [
        "https://example.com:99999",  # SplitResult.port raises ValueError
        "https://" + "a" * 64 + ".example.com",  # getaddrinfo raises UnicodeError
        "https://[2001:db8::1]:8443",  # IPv6 literal
        "https://127.0.0.1:1",  # nothing listening
    ],
)
def test_diagnose_never_raises_and_always_returns_five_checks(credentials, hostile):
    with _client(hostile, credentials) as client:
        diag = client.diagnose()

    assert [c.name for c in diag.checks] == CHECK_NAMES
    assert diag.ok is False


def test_diagnose_survives_a_transport_that_raises_a_non_http_error(credentials):
    import httpx

    def explode(request):
        raise ValueError("transport exploded")

    hostile = httpx.Client(transport=httpx.MockTransport(explode))
    with _client("https://example.test", credentials, http_client=hostile) as client:
        diag = client.diagnose()

    preflight = next(c for c in diag.checks if c.name == "api_preflight")
    assert preflight.ok is False
    assert "ValueError" in preflight.detail


def test_diagnose_survives_a_closed_client(httpserver, credentials):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])
    client = _client(httpserver.url_for("").rstrip("/"), credentials)
    client.close()

    diag = client.diagnose()

    assert [c.name for c in diag.checks] == CHECK_NAMES
    assert next(c for c in diag.checks if c.name == "api_preflight").ok is False


def test_a_base_url_with_no_host_fails_the_base_url_check(credentials):
    """`getaddrinfo("", 443)` resolves to loopback, so DNS must not be allowed to 'pass'."""
    with _client("https:///v1", credentials) as client:
        diag = client.diagnose()

    base = next(c for c in diag.checks if c.name == "base_url")
    assert base.ok is False
    assert next(c for c in diag.checks if c.name == "dns").skipped is True


def test_ipv6_literals_keep_their_brackets_in_the_report(credentials):
    with _client("https://[2001:db8::1]:8443/v1", credentials) as client:
        report = client.diagnose().report()

    assert "https://[2001:db8::1]:8443/v1" in report


# -- Response bodies and full URLs must never reach the report or the log ----


def test_response_bodies_are_never_captured(httpserver, credentials):
    secret = "SUPER-SECRET-BODY-abc123"
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json(
        {"leak": secret}, status=500
    )

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        diag = client.diagnose()

    assert secret not in diag.report()
    assert secret not in json.dumps(diag.as_dict())


def test_the_log_records_the_path_only_never_the_full_url(httpserver, credentials, caplog):
    """A base url can carry userinfo, and httpx.URL.__str__ preserves it."""
    host = httpserver.url_for("").rstrip("/").split("://", 1)[1]
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    with (
        caplog.at_level(logging.DEBUG, logger="open_banking_io"),
        _client(f"http://alice:hunter2@{host}", credentials) as client,
    ):
        client.get_accounts()

    joined = " ".join(r.getMessage() for r in caplog.records if r.name == "open_banking_io")
    assert "/api/accounts" in joined
    assert "hunter2" not in joined
    assert "alice" not in joined


def test_an_unparseable_base_url_fails_instead_of_raising(credentials):
    """urlsplit itself raises on some malformed input, so it must run behind the guard."""
    from open_banking_io import diagnostics

    diag = diagnostics.run(httpx.Client(), "https://[", "k")

    assert [c.name for c in diag.checks] == CHECK_NAMES
    assert diag.checks[0].ok is False
    assert diag.ok is False


def test_an_explicit_port_zero_is_rejected_not_silently_replaced(credentials):
    """`or` would swallow port 0 and report the scheme default, hiding the real target."""
    with _client("https://127.0.0.1:0", credentials) as client:
        base = client.diagnose().checks[0]

    assert base.ok is False
    assert "443" not in base.detail
