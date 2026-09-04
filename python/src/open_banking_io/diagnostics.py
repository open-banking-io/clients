"""Connectivity diagnostics that produce a report safe to paste into a support ticket.

Every stage is probed in order and a failure stops the ones that depend on it. Nothing here
raises: a diagnostic that blows up is useless. The report carries no API key, no private key,
no decrypted data and no response bodies -- only what is needed to tell a DNS problem from a
TLS problem from an authentication problem.
"""

from __future__ import annotations

import os
import platform
import socket
import ssl
import sys
import time
from dataclasses import dataclass, field
from importlib.metadata import PackageNotFoundError, version
from typing import Any
from urllib.parse import urlsplit, urlunsplit

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
        """Whether the SDK can actually talk to the API. Decided by the preflight through the
        configured transport, not by the direct probes -- those bypass proxies and custom TLS,
        so they can fail on a setup that works perfectly well."""
        preflight = next((c for c in self.checks if c.name == "api_preflight"), None)
        return preflight.ok if preflight is not None else False

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


def _safe_url(url: str) -> str:
    """The base url with any userinfo, query and fragment removed -- a base url may carry
    proxy or basic-auth credentials, and this string is written into the report."""
    parts = urlsplit(url)
    host = parts.hostname or ""
    if parts.port:
        host = f"{host}:{parts.port}"
    return urlunsplit((parts.scheme, host, parts.path, "", ""))


def _environment(api_key: str, base_url: str) -> dict[str, str]:
    env: dict[str, str] = {
        "sdk_version": _sdk_version(),
        "python_version": sys.version.split()[0],
        "httpx_version": _package_version("httpx"),
        "httpcore_version": _package_version("httpcore"),
        "openssl_version": ssl.OPENSSL_VERSION,
        "platform": platform.platform(),
        "api_base_url": _safe_url(base_url),
        "api_key": f"set ({len(api_key)} chars)" if api_key else "MISSING",
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
        Check(
            "base_url",
            True,
            f"{_safe_url(base_url)!r} -> host={host!r} port={port} scheme={scheme}",
        )
    )

    def skip(name: str, why: str = "an earlier stage failed") -> None:
        diag.checks.append(Check(name, False, f"not attempted ({why})", skipped=True))

    # The probes below open their own sockets, so they bypass any proxy and any custom TLS
    # configuration on the caller's httpx client. They localise a fault; they do not decide
    # the verdict -- the preflight through the configured transport does.
    with _Timer() as t:
        try:
            infos = socket.getaddrinfo(host, port, proto=socket.IPPROTO_TCP)
            addresses = sorted({str(info[4][0]) for info in infos})
            dns_ok, dns_detail = True, f"{host} -> {', '.join(addresses)}"
        except OSError as e:
            dns_ok, dns_detail = False, f"could not resolve {host!r}: {e}"
    diag.checks.append(Check("dns", dns_ok, dns_detail, t.ms))

    tcp_ok = False
    if not dns_ok:
        skip("tcp_connect")
    else:
        with _Timer() as t:
            try:
                with socket.create_connection((host, port), timeout=10.0):
                    tcp_ok, tcp_detail = True, f"connected to {host}:{port}"
            except OSError as e:
                tcp_ok, tcp_detail = False, f"could not connect to {host}:{port}: {e}"
        diag.checks.append(Check("tcp_connect", tcp_ok, tcp_detail, t.ms))

    if not tcp_ok:
        skip("tls_handshake")
    elif scheme != "https":
        diag.checks.append(Check("tls_handshake", True, "skipped (plain http)"))
    else:
        with _Timer() as t:
            try:
                ctx = ssl.create_default_context()
                # CodeQL: pin the floor so TLS 1.0/1.1 are never negotiated by the probe.
                ctx.minimum_version = ssl.TLSVersion.TLSv1_2
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

    # Always attempted, whatever the probes said: this is the request the SDK actually makes,
    # through the caller's configured transport. Only the status is reported, never the body.
    with _Timer() as t:
        try:
            resp = http.get("api/accounts")
            pre_ok = resp.status_code == 200
            pre_detail = f"GET api/accounts -> HTTP {resp.status_code}"
            if resp.status_code == 401:
                pre_detail += " (the API key was rejected)"
        except httpx.HTTPError as e:
            pre_ok, pre_detail = False, f"GET api/accounts failed: {type(e).__name__}: {e}"
    if pre_ok and not all(c.ok for c in diag.checks):
        pre_detail += " -- the request succeeded, so the failing probes above are most likely "
        pre_detail += "a proxy or custom TLS setup they bypass, not a real fault"
    diag.checks.append(Check("api_preflight", pre_ok, pre_detail, t.ms))

    return diag
