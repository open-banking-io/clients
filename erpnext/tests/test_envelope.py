# ERPNext Open Banking © 2026
# Author: open-banking.io
# Licence: MIT

"""Tests for the zero-knowledge envelope decryption.

These tests are self-contained — they generate an ephemeral keypair, encrypt
known plaintext, and verify the vendored decryptor recovers it correctly.
They mirror the envelope tests in the Python SDK and n8n node.
"""

import base64
import json

import pytest

from erpnext_open_banking.erpnext_open_banking.utils.envelope import (
    decrypt,
    decrypt_to_json,
    load_private_key,
)

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
import secrets

_VERSION = 0x01
_POINT_LEN = 65
_NONCE_LEN = 12
_TAG_LEN = 16
_HKDF_SALT = b"\x00" * 32
_HKDF_INFO = b"bank.core.ci/zk/v1"


def _generate_keypair():
    """Generates an EC P-256 keypair and returns (private_key, pkcs8_b64)."""
    priv = ec.generate_private_key(ec.SECP256R1())
    der = priv.private_bytes(
        encoding=serialization.Encoding.DER,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    )
    return priv, base64.b64encode(der).decode()


def _encrypt_envelope(recipient_pub: ec.EllipticCurvePublicKey, plaintext: bytes) -> bytes:
    """Encrypts plaintext using the OBI envelope scheme (for test fixtures)."""
    eph_priv = ec.generate_private_key(ec.SECP256R1())
    eph_pub_raw = eph_priv.public_key().public_bytes(
        encoding=serialization.Encoding.X962,
        format=serialization.PublicFormat.UncompressedPoint,
    )
    shared = eph_priv.exchange(ec.ECDH(), recipient_pub)
    key = HKDF(
        algorithm=hashes.SHA256(),
        length=32,
        salt=_HKDF_SALT,
        info=_HKDF_INFO,
    ).derive(shared)
    nonce = secrets.token_bytes(_NONCE_LEN)
    ct_and_tag = AESGCM(key).encrypt(nonce, plaintext, None)
    ciphertext = ct_and_tag[:-_TAG_LEN]
    tag = ct_and_tag[-_TAG_LEN:]
    return bytes([_VERSION]) + eph_pub_raw + nonce + tag + ciphertext


class TestEnvelope:
    def test_load_private_key_valid(self):
        """load_private_key accepts a valid PKCS#8 base64 EC key."""
        _, pkcs8_b64 = _generate_keypair()
        key = load_private_key(pkcs8_b64)
        assert isinstance(key, ec.EllipticCurvePrivateKey)

    def test_load_private_key_invalid(self):
        """load_private_key rejects non-EC keys."""
        from cryptography.hazmat.primitives.asymmetric import rsa

        rsa_priv = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        der = rsa_priv.private_bytes(
            encoding=serialization.Encoding.DER,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        )
        rsa_b64 = base64.b64encode(der).decode()
        with pytest.raises(ValueError, match="not an EC key"):
            load_private_key(rsa_b64)

    def test_decrypt_roundtrip(self):
        """decrypt recovers the original plaintext after encrypt→decrypt."""
        priv, pkcs8_b64 = _generate_keypair()
        loaded = load_private_key(pkcs8_b64)
        plaintext = b'{"iban":"DK1234567890123456","amount":"42.50"}'
        envelope_bytes = _encrypt_envelope(priv.public_key(), plaintext)
        recovered = decrypt(loaded, envelope_bytes)
        assert recovered == plaintext

    def test_decrypt_to_json(self):
        """decrypt_to_json decrypts and JSON-parses the payload."""
        priv, pkcs8_b64 = _generate_keypair()
        loaded = load_private_key(pkcs8_b64)
        payload = {"iban": "DK1234567890123456", "amount": "42.50", "name": "Checking"}
        plaintext = json.dumps(payload).encode()
        envelope_bytes = _encrypt_envelope(priv.public_key(), plaintext)
        b64 = base64.b64encode(envelope_bytes).decode()
        result = decrypt_to_json(loaded, b64)
        assert result == payload

    def test_decrypt_to_json_none(self):
        """decrypt_to_json returns None for None input."""
        priv, pkcs8_b64 = _generate_keypair()
        loaded = load_private_key(pkcs8_b64)
        assert decrypt_to_json(loaded, None) is None

    def test_decrypt_invalid_version(self):
        """decrypt rejects an envelope with wrong version byte."""
        priv, pkcs8_b64 = _generate_keypair()
        loaded = load_private_key(pkcs8_b64)
        bad = b"\x02" + b"\x00" * 100
        with pytest.raises(ValueError, match="Invalid or unsupported envelope"):
            decrypt(loaded, bad)

    def test_decrypt_too_short(self):
        """decrypt rejects an envelope shorter than the minimum header."""
        priv, pkcs8_b64 = _generate_keypair()
        loaded = load_private_key(pkcs8_b64)
        with pytest.raises(ValueError, match="Invalid or unsupported envelope"):
            decrypt(loaded, b"\x01\x00\x01")

    def test_decrypt_tampered_ciphertext(self):
        """decrypt fails AEAD authentication when the ciphertext is tampered with."""
        from cryptography.exceptions import InvalidTag

        priv, pkcs8_b64 = _generate_keypair()
        loaded = load_private_key(pkcs8_b64)
        envelope_bytes = bytearray(
            _encrypt_envelope(priv.public_key(), b'{"amount":"42.50"}')
        )
        envelope_bytes[-1] ^= 0x01  # flip one bit in the ciphertext
        with pytest.raises(InvalidTag):
            decrypt(loaded, bytes(envelope_bytes))

    def test_decrypt_tampered_tag(self):
        """decrypt fails AEAD authentication when the tag is tampered with."""
        from cryptography.exceptions import InvalidTag

        priv, pkcs8_b64 = _generate_keypair()
        loaded = load_private_key(pkcs8_b64)
        envelope_bytes = bytearray(
            _encrypt_envelope(priv.public_key(), b'{"amount":"42.50"}')
        )
        envelope_bytes[1 + _POINT_LEN + _NONCE_LEN] ^= 0x01  # flip one bit in the tag
        with pytest.raises(InvalidTag):
            decrypt(loaded, bytes(envelope_bytes))

    def test_decrypt_wrong_recipient_key(self):
        """decrypt fails when the envelope was sealed for a different key."""
        from cryptography.exceptions import InvalidTag

        priv_a, _ = _generate_keypair()
        _, pkcs8_b64_b = _generate_keypair()
        wrong_key = load_private_key(pkcs8_b64_b)
        envelope_bytes = _encrypt_envelope(priv_a.public_key(), b'{"amount":"42.50"}')
        with pytest.raises(InvalidTag):
            decrypt(wrong_key, envelope_bytes)
