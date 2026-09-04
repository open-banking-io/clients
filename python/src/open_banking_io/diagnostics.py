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
from urllib.parse import SplitResult, urlsplit, urlunsplit

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


_PROBE_TIMEOUT = 10.0


def _port(parts: SplitResult) -> int | None:
    """``SplitResult.port`` is a property that raises on an out-of-range value."""
    try:
        return parts.port
    except ValueError:
        return None


def _port_is_invalid(parts: SplitResult) -> bool:
    try:
        return parts.port == 0  # a real port is 1-65535
    except ValueError:
        return True


def _default_port(scheme: str) -> int:
    return 443 if scheme == "https" else 80


def _effective_port(port_value: int | None, scheme: str) -> int:
    """An explicit port wins, including one the scheme would otherwise imply. Only an absent
    port falls back to the scheme default -- `or` would swallow an explicit 0."""
    return _default_port(scheme) if port_value is None else port_value


def _safe_url(url: str) -> str:
    """The base url with any userinfo, query and fragment removed -- a base url may carry
    proxy or basic-auth credentials, and this string is written into the report."""
    try:
        parts = urlsplit(url)
        host = parts.hostname or ""
        if ":" in host:  # an IPv6 literal keeps its brackets or the result is unparseable
            host = f"[{host}]"
        port = _port(parts)
        if port is not None:
            host = f"{host}:{port}"
        return urlunsplit((parts.scheme, host, parts.path, "", ""))
    except Exception:
        return "<unparseable base url>"


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


_CHECK_NAMES = ("base_url", "dns", "tcp_connect", "tls_handshake", "api_preflight")


def _guard(name: str, fn: Any) -> Check:
    """Runs one stage. Any exception becomes a failed check -- a diagnostic that raises is
    useless, and it is fed exactly the malformed input a caller could not debug themselves."""
    with _Timer() as t:
        try:
            ok, detail = fn()
        except Exception as e:  # deliberate: never propagate out of diagnose()
            ok, detail = False, f"{type(e).__name__}: {e}"
    return Check(name, ok, detail, t.ms)


def run(http: httpx.Client, base_url: str, api_key: str) -> Diagnostics:
    """Probes each stage of the connection in order and collects the results.

    Never raises. Every stage is guarded, and the returned report always carries all five
    checks so a caller can index them unconditionally.
    """
    try:
        env = _environment(api_key, base_url)
    except Exception as e:
        env = {"error": f"could not collect environment: {type(e).__name__}: {e}"}

    results: dict[str, Check] = {}

    def skip(name: str, why: str = "an earlier stage failed") -> None:
        results[name] = Check(name, False, f"not attempted ({why})", skipped=True)

    # -- base url --------------------------------------------------------------
    # urlsplit itself raises on some malformed input (e.g. "https://["), so it runs behind
    # the same guard as everything else rather than ahead of it.
    try:
        parts = urlsplit(base_url)
        parse_error: str | None = None
    except ValueError as e:
        parts, parse_error = SplitResult("", "", "", "", ""), f"{type(e).__name__}: {e}"

    scheme = (parts.scheme or "").lower()
    host = parts.hostname or ""
    port_value = _port(parts)

    def check_base_url() -> tuple[bool, str]:
        if parse_error is not None:
            return False, f"could not parse the base url -- {parse_error}"
        shown = _safe_url(base_url)
        if not host:
            return False, f"{shown!r} has no host"
        if _port_is_invalid(parts):
            return False, f"{shown!r} has a port outside 1-65535"
        shown_port = _effective_port(port_value, scheme)
        return True, f"{shown!r} -> host={host!r} port={shown_port} scheme={scheme}"

    results["base_url"] = _guard("base_url", check_base_url)
    port = _effective_port(port_value, scheme)

    if not results["base_url"].ok:
        skip("dns", "the base url is unusable")
        skip("tcp_connect", "the base url is unusable")
        skip("tls_handshake", "the base url is unusable")
    else:
        # The probes below open their own sockets, so they bypass any proxy and any custom
        # TLS configuration on the caller's httpx client. They localise a fault; they do not
        # decide the verdict -- the preflight through the configured transport does.
        def check_dns() -> tuple[bool, str]:
            infos = socket.getaddrinfo(host, port, proto=socket.IPPROTO_TCP)
            addresses = sorted({str(info[4][0]) for info in infos})
            return True, f"{host} -> {', '.join(addresses)}"

        results["dns"] = _guard("dns", check_dns)

        if not results["dns"].ok:
            skip("tcp_connect")
            skip("tls_handshake")
        else:

            def check_tcp() -> tuple[bool, str]:
                with socket.create_connection((host, port), timeout=_PROBE_TIMEOUT):
                    return True, f"connected to {host}:{port}"

            results["tcp_connect"] = _guard("tcp_connect", check_tcp)

            if not results["tcp_connect"].ok:
                skip("tls_handshake")
            elif scheme != "https":
                results["tls_handshake"] = Check(
                    "tls_handshake", True, "not applicable (plain http)", skipped=True
                )
            else:

                def check_tls() -> tuple[bool, str]:
                    ctx = ssl.create_default_context()
                    # Pin the floor so the probe can never negotiate TLS 1.0/1.1.
                    ctx.minimum_version = ssl.TLSVersion.TLSv1_2
                    with (
                        socket.create_connection((host, port), timeout=_PROBE_TIMEOUT) as raw,
                        ctx.wrap_socket(raw, server_hostname=host) as tls,
                    ):
                        cert: Any = tls.getpeercert() or {}
                        issuer = "unknown"
                        for rdn in cert.get("issuer", ()):
                            for pair in rdn:
                                if len(pair) == 2 and pair[0] == "organizationName":
                                    issuer = pair[1]
                        cipher = tls.cipher()
                        return True, (
                            f"{tls.version()} {cipher[0] if cipher else ''} "
                            f"issuer={issuer!r} notAfter={cert.get('notAfter', 'unknown')}"
                        )

                results["tls_handshake"] = _guard("tls_handshake", check_tls)

    # -- preflight -------------------------------------------------------------
    # Always attempted, whatever the probes said: this is the request the SDK actually makes,
    # through the caller's configured transport. Only the status is reported, never the body.
    def check_preflight() -> tuple[bool, str]:
        resp = http.get("api/accounts")
        detail = f"GET api/accounts -> HTTP {resp.status_code}"
        if resp.status_code == 401:
            detail += " (the API key was rejected)"
        return resp.status_code == 200, detail

    preflight = _guard("api_preflight", check_preflight)
    if preflight.ok and not all(results[n].ok for n in results):
        preflight.detail += (
            " -- the request succeeded, so the failing probes above are most likely "
            "a proxy or custom TLS setup they bypass, not a real fault"
        )
    results["api_preflight"] = preflight

    return Diagnostics(
        checks=[
            results.get(n) or Check(n, False, "not attempted", skipped=True) for n in _CHECK_NAMES
        ],
        environment=env,
    )
