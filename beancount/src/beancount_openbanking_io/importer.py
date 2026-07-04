"""Beancount importer for open-banking.io (zero-knowledge PSD2 bank data).

open-banking.io returns transactions as zero-knowledge encrypted envelopes; the
official ``open-banking-io`` SDK decrypts them locally with your exported private
key. This importer is a thin mapping layer from the decrypted SDK objects onto
Beancount directives.

Usage: create a small YAML config file named ``*openbanking.yaml`` and point
``beangulp`` at it (the file itself is the "document" the importer triggers on)::

    # myfinances.openbanking.yaml
    credentials: ./open-banking.io-credentials.json   # exported from the dashboard
    accounts:
      - id: f1345859-....          # open-banking.io account id (see get_accounts())
        account: Assets:Bank:Nordea:Checking
        currency: DKK              # optional guard
        counterpart: Expenses:Uncategorized   # optional, per-account override
        assert_balance: true       # optional: emit a booked-balance assertion
"""

from __future__ import annotations

import datetime
from decimal import Decimal
from pathlib import Path
from typing import Any

import beangulp
import yaml
from beancount.core import amount, data
from beancount.core.flags import FLAG_OKAY, FLAG_WARNING

try:  # keep the module importable (docs/tests) even without the SDK installed
    from open_banking_io import OpenBankingClient
except ImportError:  # pragma: no cover
    OpenBankingClient = None

__all__ = ["OpenBankingImporter"]

_PAGE_SIZE = 500


class OpenBankingImporter(beangulp.Importer):  # type: ignore[misc]  # untyped third-party base
    """Imports open-banking.io accounts into Beancount.

    Args:
        meta_key: metadata key used to stamp each transaction's stable
            open-banking.io id (drives deduplication across re-imports).
        config_suffix: filename suffix that triggers this importer.
        counterpart: default holding account for the balancing leg; users
            reclassify it later (e.g. with smart_importer).
        client: an already-built SDK client (dependency injection for tests).
            When omitted, a client is built from the config's ``credentials`` bundle.
    """

    def __init__(
        self,
        *,
        meta_key: str = "obio-id",
        config_suffix: str = "openbanking.yaml",
        counterpart: str = "Expenses:Uncategorized",
        client: Any = None,
    ) -> None:
        self._meta_key = meta_key
        self._suffix = config_suffix
        self._counterpart = counterpart
        self._client = client

    # -- beangulp.Importer protocol -------------------------------------------

    def identify(self, filepath: str) -> bool:
        return filepath.endswith(self._suffix)

    def account(self, filepath: str) -> str:
        accounts = self._load(filepath).get("accounts") or []
        first: str = accounts[0]["account"] if accounts else "Assets:OpenBankingIO"
        return first

    def extract(self, filepath: str, existing: list[Any]) -> list[Any]:
        cfg = self._load(filepath)
        seen = self._seen_ids(existing)

        client, owns_client = self._client, False
        if client is None:
            if OpenBankingClient is None:  # pragma: no cover
                raise RuntimeError(
                    "The open-banking-io SDK is not installed. "
                    "Install it with: pip install open-banking-io"
                )
            client = OpenBankingClient.from_credentials(self._resolve(filepath, cfg["credentials"]))
            owns_client = True

        try:
            entries: list[Any] = []
            index: dict[str, Any] | None = None
            for spec in cfg.get("accounts", []):
                entries.extend(self._extract_account(filepath, client, spec, seen))
                if spec.get("assert_balance"):
                    if index is None:
                        index = self._account_index(client)
                    balance = self._balance_directive(filepath, index, spec)
                    if balance is not None:
                        entries.append(balance)
        finally:
            if owns_client and hasattr(client, "close"):
                client.close()

        entries.sort(key=lambda entry: entry.date)
        return entries

    # -- mapping ---------------------------------------------------------------

    def _extract_account(
        self, filepath: str, client: Any, spec: dict[str, Any], seen: set[str]
    ) -> list[Any]:
        obio_id = spec["id"]
        bean_account = spec["account"]
        want_ccy = spec.get("currency")
        counterpart = spec.get("counterpart", self._counterpart)

        out: list[Any] = []
        offset = 0
        lineno = 0
        while True:
            page = client.get_transactions(obio_id, limit=_PAGE_SIZE, offset=offset)
            items = list(page.items or [])
            if not items:
                break
            for txn in items:
                lineno += 1
                if txn.id in seen:
                    continue  # already in the ledger -> dedup
                directive = self._to_transaction(
                    filepath, lineno, bean_account, counterpart, txn, want_ccy
                )
                if directive is not None:
                    out.append(directive)
            offset += len(items)
            if offset >= (page.total or 0):
                break
        return out

    def _to_transaction(
        self,
        filepath: str,
        lineno: int,
        bean_account: str,
        counterpart: str,
        txn: Any,
        want_ccy: str | None,
    ) -> Any | None:
        # An unknown direction means we cannot sign the amount safely; skip rather
        # than import an outgoing payment as income (learned the hard way).
        if txn.amount is None or txn.credit_debit_indicator not in ("CRDT", "DBIT"):
            return None

        sign = Decimal(-1) if txn.credit_debit_indicator == "DBIT" else Decimal(1)
        number = sign * txn.amount
        currency = txn.currency or want_ccy or ""
        units = amount.Amount(number, currency)

        payee = (
            txn.creditor_name if txn.credit_debit_indicator == "DBIT" else txn.debtor_name
        ) or None
        narration = txn.remittance_information or txn.note or ""
        date = txn.booking_date or txn.value_date or txn.transaction_date
        flag = FLAG_OKAY if (txn.status in (None, "BOOK")) else FLAG_WARNING

        meta = data.new_metadata(filepath, lineno, {self._meta_key: txn.id})
        postings = [data.Posting(bean_account, units, None, None, None, None)]
        if counterpart:
            # Elided amount: Beancount auto-balances the second leg. Users reclassify
            # it later (e.g. with smart_importer) from the default holding account.
            postings.append(data.Posting(counterpart, None, None, None, None, None))
        return data.Transaction(
            meta, date, flag, payee, narration, data.EMPTY_SET, data.EMPTY_SET, postings
        )

    def _balance_directive(
        self, filepath: str, index: dict[str, Any], spec: dict[str, Any]
    ) -> Any | None:
        account = index.get(spec["id"])
        if account is None or not account.balances:
            return None
        booked = next((b for b in account.balances if b.type == "ITBD"), None)
        if booked is None:
            return None
        # Beancount asserts a balance at the *start* of the given day, so date it
        # tomorrow to include everything booked through today.
        date = datetime.date.today() + datetime.timedelta(days=1)
        meta = data.new_metadata(filepath, 0)
        return data.Balance(
            meta,
            date,
            spec["account"],
            amount.Amount(booked.amount, booked.currency),
            None,
            None,
        )

    # -- helpers ---------------------------------------------------------------

    def _account_index(self, client: Any) -> dict[str, Any]:
        return {account.id: account for account in client.get_accounts()}

    def _seen_ids(self, existing: list[Any]) -> set[str]:
        key = self._meta_key
        seen: set[str] = set()
        for entry in existing or []:
            if isinstance(entry, data.Transaction):
                if entry.meta and key in entry.meta:
                    seen.add(entry.meta[key])
                for posting in entry.postings:
                    if posting.meta and key in posting.meta:
                        seen.add(posting.meta[key])
        return seen

    def _load(self, filepath: str) -> dict[str, Any]:
        with open(filepath, encoding="utf-8") as handle:
            data_dict: dict[str, Any] = yaml.safe_load(handle) or {}
            return data_dict

    def _resolve(self, filepath: str, credentials: str) -> str:
        path = Path(credentials).expanduser()
        if not path.is_absolute():
            path = Path(filepath).parent / path
        return str(path)
