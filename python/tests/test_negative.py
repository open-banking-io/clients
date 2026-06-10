"""Negative-path tests: malformed envelopes, bad keys, and server/transport errors."""

import base64
import json

import httpx
import pytest
from werkzeug import Response

from open_banking_io import OpenBankingClient, envelope

API_KEY = "obk_test_3f8b9c2e1a7d4655b0e9f2c1a8d7e6f5"

# version(1) + point(65) + nonce(12) + tag(16) = 94-byte minimum before any ciphertext.
_MIN_ENVELOPE_LEN = 1 + 65 + 12 + 16


# -- Envelope decoding -------------------------------------------------------


def _priv(keypair):
    return envelope.load_private_key(keypair["privateKeyPkcs8B64"])


def test_truncated_envelope_too_short(keypair):
    priv = _priv(keypair)
    # One byte short of the fixed-header minimum: must be rejected, not index-crash.
    too_short = bytes([0x01]) + b"\x00" * (_MIN_ENVELOPE_LEN - 2)
    assert len(too_short) < _MIN_ENVELOPE_LEN
    with pytest.raises(ValueError, match="Invalid or unsupported envelope"):
        envelope.decrypt(priv, too_short)


def test_empty_envelope(keypair):
    priv = _priv(keypair)
    with pytest.raises(ValueError, match="Invalid or unsupported envelope"):
        envelope.decrypt(priv, b"")


def test_wrong_version_byte(keypair, envelopes):
    priv = _priv(keypair)
    # A full-length, otherwise-valid envelope but with an unsupported version byte.
    good = base64.b64decode(envelopes["account"])
    assert len(good) >= _MIN_ENVELOPE_LEN
    tampered = bytes([0x02]) + good[1:]
    with pytest.raises(ValueError, match="Invalid or unsupported envelope"):
        envelope.decrypt(priv, tampered)


def test_corrupted_ciphertext_fails_auth(keypair, envelopes):
    """Right length and version, but a flipped ciphertext byte must fail the GCM tag."""
    priv = _priv(keypair)
    raw = bytearray(base64.b64decode(envelopes["account"]))
    raw[-1] ^= 0xFF  # flip a ciphertext byte
    with pytest.raises(Exception):  # cryptography raises InvalidTag
        envelope.decrypt(priv, bytes(raw))


# -- Key loading -------------------------------------------------------------


def test_invalid_pkcs8_key_garbage():
    with pytest.raises(Exception):  # not valid DER / PKCS#8
        envelope.load_private_key(base64.b64encode(b"not a real key").decode())


def test_invalid_pkcs8_key_not_base64():
    with pytest.raises(Exception):
        envelope.load_private_key("@@@ this is not base64 @@@")


def test_load_private_key_rejects_non_ec_key():
    # A structurally valid PKCS#8 key that is not an EC key must be rejected.
    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric import ed25519

    key = ed25519.Ed25519PrivateKey.generate()
    der = key.private_bytes(
        encoding=serialization.Encoding.DER,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    )
    with pytest.raises(ValueError, match="not an EC key"):
        envelope.load_private_key(base64.b64encode(der).decode())


def test_decrypt_to_json_none_returns_none(keypair):
    priv = _priv(keypair)
    assert envelope.decrypt_to_json(priv, None) is None


def test_client_rejects_bad_private_key():
    with pytest.raises(Exception):
        OpenBankingClient(
            api_base_url="https://example.test",
            api_key=API_KEY,
            private_key_pkcs8=base64.b64encode(b"garbage").decode(),
        )


# -- HTTP error / malformed-JSON paths ---------------------------------------


def _client(base_url, credentials):
    return OpenBankingClient(
        api_base_url=base_url,
        api_key=credentials["apiKey"],
        private_key_pkcs8=credentials["encryptionKey"]["privateKey"],
    )


def test_server_5xx_raises(httpserver, credentials):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_response(
        Response("upstream boom", status=503, mimetype="text/plain")
    )
    with (
        _client(httpserver.url_for("").rstrip("/"), credentials) as client,
        pytest.raises(httpx.HTTPStatusError),
    ):
        client.get_accounts()


def test_malformed_json_body_raises(httpserver, credentials):
    # 200 OK but the body is not valid JSON: the client's .json() must blow up.
    httpserver.expect_request("/api/connections", method="GET").respond_with_response(
        Response("{not: valid json", status=200, mimetype="application/json")
    )
    with (
        _client(httpserver.url_for("").rstrip("/"), credentials) as client,
        pytest.raises(json.JSONDecodeError),
    ):
        client.get_connections()


def test_transactions_5xx_raises(httpserver, credentials):
    account_id = "11111111-1111-4111-8111-111111111111"
    httpserver.expect_request(
        f"/api/accounts/{account_id}/transactions", method="GET"
    ).respond_with_response(Response("server error", status=500, mimetype="text/plain"))
    with (
        _client(httpserver.url_for("").rstrip("/"), credentials) as client,
        pytest.raises(httpx.HTTPStatusError),
    ):
        client.get_transactions(account_id)
