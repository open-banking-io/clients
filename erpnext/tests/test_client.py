# ERPNext Open Banking © 2026
# Author: open-banking.io
# Licence: MIT

"""Tests for the API client's credential parsing and value mapping.

Network calls are not exercised here (that needs a live API); these tests
ensure the module imports cleanly without frappe and that the pure logic —
bundle parsing, URL handling, decimal parsing — is correct. CI previously
never imported client.py at all.
"""

import base64
import json

import pytest

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec

from erpnext_open_banking.erpnext_open_banking.utils.client import (
    OpenBankingClient,
    _parse_optional_decimal,
)


def _private_key_b64() -> str:
    priv = ec.generate_private_key(ec.SECP256R1())
    der = priv.private_bytes(
        encoding=serialization.Encoding.DER,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    )
    return base64.b64encode(der).decode()


def _bundle(**overrides) -> str:
    bundle = {
        "apiBaseUrl": "http://localhost:8081/",
        "apiKey": "test-key",
        "encryptionKey": {"privateKey": _private_key_b64()},
    }
    bundle.update(overrides)
    return json.dumps(bundle)


class TestFromCredentials:
    def test_valid_bundle(self):
        client = OpenBankingClient.from_credentials(_bundle())
        assert client._base_url == "http://localhost:8081"
        client.close()

    def test_base_url_override_wins(self):
        client = OpenBankingClient.from_credentials(
            _bundle(), base_url_override="https://staging.example/"
        )
        assert client._base_url == "https://staging.example"
        client.close()

    def test_invalid_json_rejected(self):
        with pytest.raises(ValueError, match="not valid JSON"):
            OpenBankingClient.from_credentials("not json")

    def test_non_object_bundle_rejected(self):
        with pytest.raises(ValueError, match="JSON object"):
            OpenBankingClient.from_credentials('["a", "list"]')

    def test_missing_api_key_rejected(self):
        raw = json.loads(_bundle())
        del raw["apiKey"]
        with pytest.raises(ValueError, match="no apiKey"):
            OpenBankingClient.from_credentials(json.dumps(raw))

    def test_missing_private_key_rejected(self):
        raw = json.loads(_bundle())
        raw["encryptionKey"] = {}
        with pytest.raises(ValueError, match="no encryption private key"):
            OpenBankingClient.from_credentials(json.dumps(raw))

    def test_file_path_is_not_read(self):
        """A path must be treated as (invalid) JSON, not opened — no
        local-file-read primitive via the Settings field."""
        with pytest.raises(ValueError, match="not valid JSON"):
            OpenBankingClient.from_credentials("/etc/hosts")


class TestParseOptionalDecimal:
    def test_none_and_empty_preserved(self):
        assert _parse_optional_decimal(None) is None
        assert _parse_optional_decimal("") is None

    def test_string_passthrough(self):
        assert _parse_optional_decimal("828.13") == "828.13"

    def test_float_parsed_losslessly(self):
        assert _parse_optional_decimal(0.1) == "0.1"

    def test_int_parsed(self):
        assert _parse_optional_decimal(7) == "7"
