"""Unit tests for helpers, constructor validation, and the from_credentials factory."""

import json
from datetime import date, datetime, timezone
from decimal import Decimal
from pathlib import Path

import httpx
import pytest

from open_banking_io import OpenBankingClient
from open_banking_io.client import (
    _parse_date,
    _parse_datetime,
    _parse_decimal,
    _parse_decimal_nullable,
    _user_agent,
)

API_KEY = "obk_test_3f8b9c2e1a7d4655b0e9f2c1a8d7e6f5"


# -- Parsing helpers ---------------------------------------------------------


def test_user_agent_format():
    ua = _user_agent()
    assert ua.startswith("open-banking-io/python/")


def test_user_agent_unknown_when_package_missing(monkeypatch):
    def boom(_name):
        from importlib.metadata import PackageNotFoundError

        raise PackageNotFoundError(_name)

    monkeypatch.setattr("open_banking_io.client.version", boom)
    assert _user_agent() == "open-banking-io/python/unknown"


def test_version_matches_package_metadata():
    """__version__ must agree with the version declared in pyproject.toml."""
    from importlib.metadata import version as pkg_version

    import open_banking_io

    assert open_banking_io.__version__ == pkg_version("open-banking-io")


def test_parse_date_none_and_value():
    assert _parse_date(None) is None
    assert _parse_date("") is None
    assert _parse_date("2026-06-11") == date(2026, 6, 11)


def test_parse_datetime_none():
    assert _parse_datetime(None) is None
    assert _parse_datetime("") is None


def test_parse_datetime_with_offset():
    parsed = _parse_datetime("2026-06-11T12:30:00+00:00")
    assert parsed == datetime(2026, 6, 11, 12, 30, tzinfo=timezone.utc)


def test_parse_datetime_normalizes_trailing_z():
    parsed = _parse_datetime("2026-06-11T12:30:00Z")
    assert parsed == datetime(2026, 6, 11, 12, 30, tzinfo=timezone.utc)


def test_parse_decimal_defaults_to_zero():
    assert _parse_decimal(None) == Decimal(0)
    assert _parse_decimal("") == Decimal(0)
    assert _parse_decimal("12.34") == Decimal("12.34")


def test_parse_decimal_nullable():
    assert _parse_decimal_nullable(None) is None
    assert _parse_decimal_nullable("") is None
    assert _parse_decimal_nullable("9.99") == Decimal("9.99")


# -- Constructor validation --------------------------------------------------


def _priv(credentials):
    return credentials["encryptionKey"]["privateKey"]


def test_constructor_rejects_blank_base_url(credentials):
    with pytest.raises(ValueError, match="api_base_url is required"):
        OpenBankingClient(
            api_base_url="   ",
            api_key=API_KEY,
            private_key_pkcs8=_priv(credentials),
        )


def test_constructor_rejects_blank_api_key(credentials):
    with pytest.raises(ValueError, match="api_key is required"):
        OpenBankingClient(
            api_base_url="https://example.test",
            api_key="  ",
            private_key_pkcs8=_priv(credentials),
        )


def test_constructor_rejects_blank_private_key():
    with pytest.raises(ValueError, match="private_key_pkcs8 is required"):
        OpenBankingClient(
            api_base_url="https://example.test",
            api_key=API_KEY,
            private_key_pkcs8="   ",
        )


# -- from_credentials factory ------------------------------------------------


def test_from_credentials_from_json_string(credentials):
    raw = json.dumps(credentials)
    with OpenBankingClient.from_credentials(raw) as client:
        assert client._http.headers["X-Api-Key"] == credentials["apiKey"]
        assert str(client._http.base_url).startswith(credentials["apiBaseUrl"])


def test_from_credentials_from_file_path(fixtures_dir):
    path = str(fixtures_dir / "credentials.json")
    with OpenBankingClient.from_credentials(path) as client:
        assert client._http.headers["X-Api-Key"] == API_KEY


def test_from_credentials_missing_file_treated_as_json():
    # A non-existent path is not a file, so it is parsed as JSON -> JSON error.
    with pytest.raises(json.JSONDecodeError):
        OpenBankingClient.from_credentials("/no/such/credentials/bundle.json")


def test_from_credentials_malformed_json():
    with pytest.raises(json.JSONDecodeError):
        OpenBankingClient.from_credentials("{not valid json")


def test_from_credentials_missing_api_key(credentials):
    bundle = json.loads(json.dumps(credentials))
    bundle.pop("apiKey")
    with pytest.raises(ValueError, match="no apiKey"):
        OpenBankingClient.from_credentials(json.dumps(bundle))


def test_from_credentials_missing_encryption_key(credentials):
    bundle = json.loads(json.dumps(credentials))
    bundle.pop("encryptionKey")
    with pytest.raises(ValueError, match="no encryption private key"):
        OpenBankingClient.from_credentials(json.dumps(bundle))


def test_from_credentials_accepts_pkcs8_b64_alias(credentials, keypair):
    # The factory also accepts the `privateKeyPkcs8B64` key as a fallback.
    bundle = {
        "apiBaseUrl": credentials["apiBaseUrl"],
        "apiKey": API_KEY,
        "encryptionKey": {"privateKeyPkcs8B64": keypair["privateKeyPkcs8B64"]},
    }
    with OpenBankingClient.from_credentials(json.dumps(bundle)) as client:
        assert client._http.headers["X-Api-Key"] == API_KEY


def test_from_credentials_file_path_roundtrip(tmp_path: Path, credentials):
    path = tmp_path / "creds.json"
    path.write_text(json.dumps(credentials), encoding="utf-8")
    with OpenBankingClient.from_credentials(str(path)) as client:
        assert client._http.headers["X-Api-Key"] == credentials["apiKey"]


# -- Timeout -----------------------------------------------------------------


def test_default_http_client_uses_default_timeout(credentials):
    # No timeout passed -> the default httpx.Client keeps the 30s default.
    with OpenBankingClient(
        api_base_url="https://example.test",
        api_key=API_KEY,
        private_key_pkcs8=_priv(credentials),
    ) as client:
        assert client._http.timeout == httpx.Timeout(30.0)


def test_default_http_client_honors_custom_timeout(credentials):
    # With no injected client, `timeout=` is applied to the default httpx.Client.
    with OpenBankingClient(
        api_base_url="https://example.test",
        api_key=API_KEY,
        private_key_pkcs8=_priv(credentials),
        timeout=5.0,
    ) as client:
        assert client._http.timeout == httpx.Timeout(5.0)


def test_timeout_ignored_when_http_client_injected(credentials):
    # An injected client is used as-is; the constructor must not override its timeout.
    injected = httpx.Client(timeout=httpx.Timeout(12.0))
    client = OpenBankingClient(
        api_base_url="https://example.test",
        api_key=API_KEY,
        private_key_pkcs8=_priv(credentials),
        http_client=injected,
        timeout=5.0,
    )
    assert client._http is injected
    assert injected.timeout == httpx.Timeout(12.0)
    injected.close()


def test_from_credentials_threads_timeout(credentials):
    raw = json.dumps(credentials)
    with OpenBankingClient.from_credentials(raw, timeout=7.5) as client:
        assert client._http.timeout == httpx.Timeout(7.5)


# -- Lifecycle ---------------------------------------------------------------


def test_close_does_not_close_injected_http_client(credentials):
    # When the caller supplies the http client, close() must leave it open.
    injected = httpx.Client()
    client = OpenBankingClient(
        api_base_url="https://example.test",
        api_key=API_KEY,
        private_key_pkcs8=credentials["encryptionKey"]["privateKey"],
        http_client=injected,
    )
    client.close()
    assert not injected.is_closed
    injected.close()
