"""open-banking.io Python client.

Server-to-server client that authenticates with an API key and decrypts the
zero-knowledge data envelopes locally with your exported private key.
"""

from .client import OpenBankingClient
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
    "OpenBankingClient",
    "Account",
    "Balance",
    "Transaction",
    "TransactionPage",
    "Connection",
    "SyncResult",
    "SyncAllResult",
]

__version__ = "0.1.0"
