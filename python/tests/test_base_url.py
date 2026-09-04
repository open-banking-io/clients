"""api_base_url normalization: whitespace and a missing scheme must not surface as
transport errors on the first request."""

import httpx
import pytest

from open_banking_io import OpenBankingClient

API_KEY = "obk_test_3f8b9c2e1a7d4655b0e9f2c1a8d7e6f5"


def _client(base_url, credentials):
    return OpenBankingClient(
        api_base_url=base_url,
        api_key=credentials["apiKey"],
        private_key_pkcs8=credentials["encryptionKey"]["privateKey"],
    )


@pytest.mark.parametrize(
    "padded",
    [
        "  {url}",
        "{url}  ",
        "\t{url}\n",
        "\n{url}",
        "  {url}/  ",
    ],
)
def test_whitespace_padded_base_url_still_reaches_the_server(httpserver, credentials, padded):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_json([])

    base = httpserver.url_for("").rstrip("/")
    with _client(padded.format(url=base), credentials) as client:
        assert client.get_accounts() == []


def test_whitespace_is_stripped_from_base_url(credentials):
    with _client("  https://open-banking.io  ", credentials) as client:
        assert str(client._http.base_url) == "https://open-banking.io/"


@pytest.mark.parametrize(
    "bad",
    ["open-banking.io", "//open-banking.io", "ftp://open-banking.io"],
)
def test_base_url_without_http_scheme_is_rejected_at_construction(credentials, bad):
    with pytest.raises(ValueError, match="http"):
        _client(bad, credentials)


def test_scheme_error_is_not_deferred_to_a_transport_error(credentials):
    """Regression: a scheme-less base URL used to raise httpx.UnsupportedProtocol -- a
    TransportError -- from the first call, long after the bad value was accepted."""
    try:
        _client("open-banking.io", credentials)
    except ValueError:
        return
    except httpx.TransportError as exc:  # pragma: no cover
        pytest.fail(f"bad base URL deferred to a transport error: {exc!r}")
    pytest.fail("a scheme-less base URL was accepted")


def test_from_credentials_strips_a_padded_api_base_url(credentials, tmp_path):
    import json

    bundle = dict(credentials)
    bundle["apiBaseUrl"] = " https://open-banking.io\n"
    path = tmp_path / "bundle.json"
    path.write_text(json.dumps(bundle))

    with OpenBankingClient.from_credentials(str(path)) as client:
        assert str(client._http.base_url) == "https://open-banking.io/"


# -- Cleartext and non-string base URLs ---------------------------------------


@pytest.mark.parametrize(
    "loopback",
    [
        "http://localhost:8080",
        "http://127.0.0.1:8080",
        "http://[::1]:8080",
        "HTTP://LOCALHOST:8080",
    ],
)
def test_cleartext_http_is_allowed_on_loopback(credentials, loopback):
    with _client(loopback, credentials) as client:
        assert client._http.base_url.scheme == "http"


@pytest.mark.parametrize(
    "remote",
    [
        "http://open-banking.io",
        "http://192.168.1.10:8080",
        "http://localhost.evil.test",
    ],
)
def test_cleartext_http_to_a_remote_host_is_rejected(credentials, remote):
    """X-Api-Key and decrypted session uids must never cross the network in the clear."""
    with pytest.raises(ValueError, match="https"):
        _client(remote, credentials)


@pytest.mark.parametrize("bad", [123, 12.5, True, [], {}, object()])
def test_non_string_api_base_url_raises_value_error(credentials, bad):
    with pytest.raises(ValueError, match="api_base_url"):
        _client(bad, credentials)


def test_from_credentials_rejects_a_non_string_api_base_url(credentials, tmp_path):
    import json

    bundle = dict(credentials)
    bundle["apiBaseUrl"] = 1234
    path = tmp_path / "bundle.json"
    path.write_text(json.dumps(bundle))

    with pytest.raises(ValueError, match="api_base_url"):
        OpenBankingClient.from_credentials(str(path))
