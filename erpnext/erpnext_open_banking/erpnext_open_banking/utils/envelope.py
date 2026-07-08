# ERPNext Open Banking © 2026
# Author: open-banking.io
# Licence: MIT


"""Decrypts open-banking.io's zero-knowledge data envelopes.

Self-contained port of the open-banking.io Python SDK ``envelope.py``.
Uses only ``cryptography`` (already a Frappe dependency) — no external
packages needed, so ``bench get-app`` just works.

Scheme: ephemeral ECDH on NIST P-256 → HKDF-SHA256 → AES-256-GCM.
Wire: ``version(1)=0x01 | ephemeralPublicKeyRaw(65) | nonce(12) | tag(16) | ciphertext``.
Only the user's private key can decrypt — the service stores ciphertext it cannot read.
"""

from __future__ import annotations

import base64
import json
from typing import Any

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives.kdf.hkdf import HKDF

_VERSION = 0x01
_POINT_LEN = 65
_NONCE_LEN = 12
_TAG_LEN = 16
_HKDF_SALT = b"\x00" * 32
_HKDF_INFO = b"bank.core.ci/zk/v1"


def load_private_key(private_key_pkcs8_b64: str) -> ec.EllipticCurvePrivateKey:
    """Loads a base64 PKCS#8 EC (SECP256R1) private key."""
    der = base64.b64decode(private_key_pkcs8_b64)
    key = serialization.load_der_private_key(der, password=None)
    if not isinstance(key, ec.EllipticCurvePrivateKey):
        raise ValueError("Private key is not an EC key")
    return key


def decrypt(private_key: ec.EllipticCurvePrivateKey, envelope: bytes) -> bytes:
    """Decrypts the raw bytes of a zero-knowledge envelope."""
    if len(envelope) < 1 + _POINT_LEN + _NONCE_LEN + _TAG_LEN or envelope[0] != _VERSION:
        raise ValueError("Invalid or unsupported envelope")

    eph_pub_bytes = envelope[1 : 1 + _POINT_LEN]
    nonce = envelope[1 + _POINT_LEN : 1 + _POINT_LEN + _NONCE_LEN]
    tag = envelope[1 + _POINT_LEN + _NONCE_LEN : 1 + _POINT_LEN + _NONCE_LEN + _TAG_LEN]
    ciphertext = envelope[1 + _POINT_LEN + _NONCE_LEN + _TAG_LEN :]

    eph_pub = ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256R1(), eph_pub_bytes)
    shared = private_key.exchange(ec.ECDH(), eph_pub)

    key = HKDF(
        algorithm=hashes.SHA256(),
        length=32,
        salt=_HKDF_SALT,
        info=_HKDF_INFO,
    ).derive(shared)

    return AESGCM(key).decrypt(nonce, ciphertext + tag, None)


def decrypt_to_json(
    private_key: ec.EllipticCurvePrivateKey, envelope_b64: str | None
) -> dict[str, Any] | None:
    """Decrypts a base64 envelope and parses its JSON payload. ``None`` in → ``None``."""
    if envelope_b64 is None:
        return None
    plaintext = decrypt(private_key, base64.b64decode(envelope_b64))
    payload: dict[str, Any] = json.loads(plaintext)
    return payload
