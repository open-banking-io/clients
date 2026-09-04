"""open-banking.io Python client.

Server-to-server client that authenticates with an API key and decrypts the
zero-knowledge data envelopes locally with your exported private key.
"""

from importlib.metadata import PackageNotFoundError
from importlib.metadata import version as _pkg_version

from .client import OpenBankingClient
from .diagnostics import Check, Diagnostics
from .models import (
    Account,
    Balance,
    Connection,
    SyncAllResult,
    SyncResult,
    Transaction,
    TransactionPage,
)

__all__ = [
    "Account",
    "Balance",
    "Check",
    "Connection",
    "Diagnostics",
    "OpenBankingClient",
    "SyncAllResult",
    "SyncResult",
    "Transaction",
    "TransactionPage",
]

try:
    __version__ = _pkg_version("open-banking-io")
except PackageNotFoundError:  # pragma: no cover
    __version__ = "0.0.0.dev0"
