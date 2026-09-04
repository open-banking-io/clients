"""Connectivity diagnostics that produce a report safe to paste into a support ticket.

Every stage is probed in order and a failure stops the ones that depend on it. Nothing here
raises: a diagnostic that blows up is useless. The report carries no API key, no private key,
no decrypted data and no response bodies -- only what is needed to tell a DNS problem from a
TLS problem from an authentication problem.
"""

from __future__ import annotations

import hashlib
import os
import platform
import socket
import ssl
import sys
import time
from dataclasses import dataclass, field
from importlib.metadata import PackageNotFoundError, version
from typing import Any
from urllib.parse import urlsplit

import httpx

# Environment variables worth reporting the presence of. Values can carry proxy
# credentials, so only the name is ever emitted.
_ENV_VARS_OF_INTEREST = (
    "HTTP_PROXY",
    "HTTPS_PROXY",
    "ALL_PROXY",
    "NO_PROXY",
    "http_proxy",
    "https_proxy",
    "all_proxy",
    "no_proxy",
    "SSL_CERT_FILE",
    "SSL_CERT_DIR",
    "REQUESTS_CA_BUNDLE",
    "CURL_CA_BUNDLE",
)


@dataclass
class Check:
    """One diagnostic stage."""

    name: str
    ok: bool
    detail: str
    duration_ms: float = 0.0
    skipped: bool = False

    def as_dict(self) -> dict[str, Any]:
        return {
            "name": self.name,
            "ok": self.ok,
            "detail": self.detail,
            "duration_ms": round(self.duration_ms, 1),
            "skipped": self.skipped,
        }


@dataclass
class Diagnostics:
    """The result of :meth:`OpenBankingClient.diagnose`."""

    checks: list[Check] = field(default_factory=list)
    environment: dict[str, str] = field(default_factory=dict)

    @property
    def ok(self) -> bool:
        return all(c.ok for c in self.checks)

    def as_dict(self) -> dict[str, Any]:
        return {
            "ok": self.ok,
            "checks": [c.as_dict() for c in self.checks],
            "environment": dict(self.environment),
        }

    def report(self) -> str:
        """A pasteable plain-text summary. Contains no secrets."""
        lines = ["open-banking.io diagnostics", "=" * 27, ""]
        for c in self.checks:
            mark = "SKIP" if c.skipped else ("PASS" if c.ok else "FAIL")
            lines.append(f"[{mark}] {c.name:<14} {c.detail}  ({c.duration_ms:.0f} ms)")
        lines += ["", "Environment", "-" * 11]
        for key, value in self.environment.items():
            lines.append(f"  {key}: {value}")
        lines += ["", f"Result: {'ok' if self.ok else 'FAILED'}"]
        return "\n".join(lines)

    def __str__(self) -> str:
        return self.report()


def _fingerprint(secret: str) -> str:
    """A one-way, non-reversible handle support can correlate against, never the value."""
    return hashlib.sha256(secret.encode("utf-8")).hexdigest()[:16]


def _sdk_version() -> str:
    try:
        return version("open-banking-io")
    except PackageNotFoundError:
        return "unknown"


def _package_version(name: str) -> str:
    try:
        return version(name)
    except PackageNotFoundError:
        return "unknown"


def _environment(api_key: str, base_url: str) -> dict[str, str]:
    env: dict[str, str] = {
        "sdk_version": _sdk_version(),
        "python_version": sys.version.split()[0],
        "httpx_version": _package_version("httpx"),
        "httpcore_version": _package_version("httpcore"),
        "openssl_version": ssl.OPENSSL_VERSION,
        "platform": platform.platform(),
        "api_base_url": base_url,
        "api_key_fingerprint": _fingerprint(api_key),
    }
    present = [name for name in _ENV_VARS_OF_INTEREST if os.environ.get(name)]
    env["env_vars_set"] = ", ".join(present) if present else "(none)"
    return env


class _Timer:
    def __enter__(self) -> _Timer:
        self._start = time.perf_counter()
        return self

    def __exit__(self, *exc: object) -> None:
        self.ms = (time.perf_counter() - self._start) * 1000.0

    ms: float = 0.0


def run(http: httpx.Client, base_url: str, api_key: str) -> Diagnostics:
    """Probes each stage of the connection in order and collects the results."""
    diag = Diagnostics(environment=_environment(api_key, base_url))

    parts = urlsplit(base_url)
    scheme = (parts.scheme or "").lower()
    host = parts.hostname or ""
    port = parts.port or (443 if scheme == "https" else 80)

    diag.checks.append(
        Check("base_url", True, f"{base_url!r} -> host={host!r} port={port} scheme={scheme}")
    )

    def skip(name: str) -> None:
        diag.checks.append(
            Check(name, False, "not attempted (an earlier stage failed)", skipped=True)
        )

    # DNS
    addresses: list[str] = []
    with _Timer() as t:
        try:
            infos = socket.getaddrinfo(host, port, proto=socket.IPPROTO_TCP)
            addresses = sorted({str(info[4][0]) for info in infos})
            dns_ok, dns_detail = True, f"{host} -> {', '.join(addresses)}"
        except OSError as e:
            dns_ok, dns_detail = False, f"could not resolve {host!r}: {e}"
    diag.checks.append(Check("dns", dns_ok, dns_detail, t.ms))
    if not dns_ok:
        skip("tcp_connect")
        skip("tls_handshake")
        skip("api_preflight")
        return diag

    # TCP
    with _Timer() as t:
        try:
            with socket.create_connection((host, port), timeout=10.0):
                tcp_ok, tcp_detail = True, f"connected to {host}:{port}"
        except OSError as e:
            tcp_ok, tcp_detail = False, f"could not connect to {host}:{port}: {e}"
    diag.checks.append(Check("tcp_connect", tcp_ok, tcp_detail, t.ms))
    if not tcp_ok:
        skip("tls_handshake")
        skip("api_preflight")
        return diag

    # TLS
    with _Timer() as t:
        if scheme != "https":
            tls_ok, tls_detail = True, "skipped (plain http)"
        else:
            try:
                ctx = ssl.create_default_context()
                with (
                    socket.create_connection((host, port), timeout=10.0) as raw,
                    ctx.wrap_socket(raw, server_hostname=host) as tls,
                ):
                    cert: Any = tls.getpeercert() or {}
                    issuer = "unknown"
                    for rdn in cert.get("issuer", ()):
                        for key, value in rdn:
                            if key == "organizationName":
                                issuer = value
                    cipher = tls.cipher()
                    tls_ok = True
                    tls_detail = (
                        f"{tls.version()} {cipher[0] if cipher else ''} "
                        f"issuer={issuer!r} notAfter={cert.get('notAfter', 'unknown')}"
                    )
            except (OSError, ssl.SSLError) as e:
                tls_ok, tls_detail = False, f"TLS handshake failed: {e}"
    diag.checks.append(Check("tls_handshake", tls_ok, tls_detail, t.ms))
    if not tls_ok:
        skip("api_preflight")
        return diag

    # An authenticated request. Only the status is reported -- never the body.
    with _Timer() as t:
        try:
            resp = http.get("api/accounts")
            pre_ok = resp.status_code == 200
            pre_detail = f"GET api/accounts -> HTTP {resp.status_code}"
            if resp.status_code == 401:
                pre_detail += " (the API key was rejected)"
        except httpx.HTTPError as e:
            pre_ok, pre_detail = False, f"GET api/accounts failed: {type(e).__name__}: {e}"
    diag.checks.append(Check("api_preflight", pre_ok, pre_detail, t.ms))

    return diag
