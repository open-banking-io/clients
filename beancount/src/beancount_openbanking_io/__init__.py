"""Beancount importer for open-banking.io — zero-knowledge PSD2 bank sync.

Pulls balances and transactions from open-banking.io (decrypted locally by the
official ``open-banking-io`` SDK) and maps them onto Beancount directives.
"""

from importlib.metadata import PackageNotFoundError
from importlib.metadata import version as _pkg_version

from beancount_openbanking_io.importer import OpenBankingImporter

try:
    __version__ = _pkg_version("beancount-openbanking-io")
except PackageNotFoundError:  # pragma: no cover - source checkout without install
    __version__ = "0.0.0"

__all__ = ["OpenBankingImporter", "__version__"]
