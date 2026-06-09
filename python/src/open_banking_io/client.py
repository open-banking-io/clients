"""Server-to-server client for open-banking.io.

Authenticates with an API key (``X-Api-Key``) and decrypts the zero-knowledge data
envelopes locally with the exported private key -- the service only ever returns
ciphertext it cannot read.
"""

from __future__ import annotations

import json
import os
from datetime import date, datetime
from decimal import Decimal
from typing import Any

import httpx

from . import envelope
from .models import (
    Account,
    Balance,
    Connection,
    SyncAllResult,
    SyncResult,
    Transaction,
    TransactionPage,
)


def _parse_date(value: str | None) -> date | None:
    if not value:
        return None
    return date.fromisoformat(value)


def _parse_datetime(value: str | None) -> datetime | None:
    if not value:
        return None
    # Normalize a trailing 'Z' which datetime.fromisoformat handles only on 3.11+.
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    return datetime.fromisoformat(value)


def _parse_decimal(value: str | None) -> Decimal:
    if value is None or value == "":
        return Decimal(0)
    return Decimal(value)


def _parse_decimal_nullable(value: str | None) -> Decimal | None:
    if value is None or value == "":
        return None
    return Decimal(value)


class OpenBankingClient:
    """Decrypting client for the open-banking.io API."""

    def __init__(
        self,
        api_base_url: str,
        api_key: str,
        private_key_pkcs8: str,
        http_client: httpx.Client | None = None,
    ) -> None:
        if not api_base_url or not api_base_url.strip():
            raise ValueError("api_base_url is required")
        if not api_key or not api_key.strip():
            raise ValueError("api_key is required")
        if not private_key_pkcs8 or not private_key_pkcs8.strip():
            raise ValueError("private_key_pkcs8 is required")

        self._private_key = envelope.load_private_key(private_key_pkcs8)
        self._owns_http = http_client is None
        self._http = http_client or httpx.Client()
        self._http.base_url = httpx.URL(api_base_url.rstrip("/") + "/")
        self._http.headers["X-Api-Key"] = api_key

    # -- Construction ----------------------------------------------------------

    @classmethod
    def from_credentials(
        cls, path_or_json: str, http_client: httpx.Client | None = None
    ) -> "OpenBankingClient":
        """Builds a client from a credentials-bundle JSON string or a path to a bundle file."""
        if os.path.exists(path_or_json):
            with open(path_or_json, "r", encoding="utf-8") as fh:
                raw = fh.read()
        else:
            raw = path_or_json

        bundle = json.loads(raw)
        api_base_url = bundle.get("apiBaseUrl", "")
        api_key = bundle.get("apiKey")
        if not api_key:
            raise ValueError("The credentials bundle has no apiKey")

        enc_key = bundle.get("encryptionKey") or {}
        private_key = enc_key.get("privateKey") or enc_key.get("privateKeyPkcs8B64")
        if not private_key:
            raise ValueError("The credentials bundle has no encryption private key")

        return cls(api_base_url, api_key, private_key, http_client)

    # -- Public API ------------------------------------------------------------

    def get_accounts(self) -> list[Account]:
        """Lists the user's accounts with all sensitive fields decrypted."""
        wires = self._get_account_wires()
        return [self._map_account(w) for w in wires]

    def get_transactions(
        self,
        account_id: str,
        *,
        date_from: date | str | None = None,
        date_to: date | str | None = None,
        limit: int | None = None,
        offset: int | None = None,
    ) -> TransactionPage:
        """Returns a page of an account's statement, newest first, with decrypted fields."""
        params: dict[str, Any] = {}
        if date_from is not None:
            params["from"] = date_from.isoformat() if isinstance(date_from, date) else date_from
        if date_to is not None:
            params["to"] = date_to.isoformat() if isinstance(date_to, date) else date_to
        if limit is not None:
            params["limit"] = limit
        if offset is not None:
            params["offset"] = offset

        resp = self._http.get(f"api/accounts/{account_id}/transactions", params=params)
        resp.raise_for_status()
        page = resp.json()

        items = [self._map_transaction(t) for t in page.get("items", [])]
        return TransactionPage(items=items, total=page.get("total", 0))

    def get_connections(self) -> list[Connection]:
        """Lists the user's bank connections."""
        resp = self._http.get("api/connections")
        resp.raise_for_status()
        return [
            Connection(
                session_id=c.get("sessionId", ""),
                aspsp_name=c.get("aspspName", ""),
                aspsp_country=c.get("aspspCountry", ""),
                valid_until=_parse_datetime(c.get("validUntil")),
                status=c.get("status", ""),
                account_count=c.get("accountCount", 0),
                last_synced_at=_parse_datetime(c.get("lastSyncedAt")),
                psu_type=c.get("psuType"),
            )
            for c in resp.json()
        ]

    def sync(self, account_id: str) -> SyncResult:
        """Triggers an online sync of one account.

        Decrypts that account's Enable Banking uid and posts it, so the service can
        fetch fresh data without ever holding the uid in plaintext.
        """
        wires = self._get_account_wires()
        account = next((a for a in wires if a.get("id") == account_id), None)
        if account is None:
            raise ValueError(f"Account {account_id} not found")
        uid = self._decrypt_uid(account)
        if uid is None:
            raise ValueError(
                "Account has no active session (reconnect required) -- cannot sync"
            )

        resp = self._http.post(f"api/accounts/{account_id}/sync", json={"uid": uid})
        resp.raise_for_status()
        result = resp.json()
        return SyncResult(
            new_transactions=result.get("newTransactions", 0),
            total_fetched=result.get("totalFetched", 0),
        )

    def sync_all(self) -> SyncAllResult:
        """Triggers an online sync of every account that has an active session."""
        wires = self._get_account_wires()
        items = []
        for a in wires:
            uid = self._decrypt_uid(a)
            if uid is not None:
                items.append({"accountId": a.get("id"), "uid": uid})

        resp = self._http.post("api/sync", json={"items": items})
        resp.raise_for_status()
        result = resp.json()
        return SyncAllResult(
            accounts=result.get("accounts", 0),
            new_transactions=result.get("newTransactions", 0),
        )

    # -- Internals -------------------------------------------------------------

    def _get_account_wires(self) -> list[dict[str, Any]]:
        resp = self._http.get("api/accounts")
        resp.raise_for_status()
        return resp.json()

    def _decrypt_uid(self, account: dict[str, Any]) -> str | None:
        payload = envelope.decrypt_to_json(self._private_key, account.get("uidEnc"))
        return payload.get("uid") if payload else None

    def _map_account(self, a: dict[str, Any]) -> Account:
        acc = envelope.decrypt_to_json(self._private_key, a.get("enc")) or {}
        name = envelope.decrypt_to_json(self._private_key, a.get("displayNameEnc")) or {}

        balances = []
        for b in a.get("balances", []):
            dec = envelope.decrypt_to_json(self._private_key, b.get("enc")) or {}
            balances.append(
                Balance(
                    type=b.get("type", ""),
                    currency=b.get("currency", ""),
                    reference_date=_parse_date(b.get("referenceDate")),
                    name=dec.get("name"),
                    amount=_parse_decimal(dec.get("amount")),
                )
            )

        return Account(
            id=a.get("id", ""),
            aspsp_name=a.get("aspspName", ""),
            aspsp_country=a.get("aspspCountry", ""),
            currency=a.get("currency", ""),
            account_type=a.get("accountType"),
            bic=a.get("bic"),
            needs_reconnect=a.get("needsReconnect", False),
            iban=acc.get("iban"),
            bban=acc.get("bban"),
            owner_name=acc.get("ownerName"),
            account_name=acc.get("accountName"),
            product=acc.get("product"),
            display_name=name.get("displayName"),
            balances=balances,
        )

    def _map_transaction(self, t: dict[str, Any]) -> Transaction:
        d = envelope.decrypt_to_json(self._private_key, t.get("enc")) or {}
        return Transaction(
            id=t.get("id", ""),
            currency=t.get("currency", ""),
            credit_debit_indicator=t.get("creditDebitIndicator", ""),
            status=t.get("status"),
            booking_date=_parse_date(t.get("bookingDate")),
            value_date=_parse_date(t.get("valueDate")),
            transaction_date=_parse_date(t.get("transactionDate")),
            bank_transaction_code=t.get("bankTransactionCode"),
            amount=_parse_decimal(d.get("amount")),
            creditor_name=d.get("creditorName"),
            creditor_iban=d.get("creditorIban"),
            creditor_bban=d.get("creditorBban"),
            creditor_agent_bic=d.get("creditorAgentBic"),
            debtor_name=d.get("debtorName"),
            debtor_iban=d.get("debtorIban"),
            debtor_bban=d.get("debtorBban"),
            debtor_agent_bic=d.get("debtorAgentBic"),
            remittance_information=d.get("remittanceInformation"),
            note=d.get("note"),
            reference_number=d.get("referenceNumber"),
            exchange_rate=d.get("exchangeRate"),
            merchant_category_code=d.get("merchantCategoryCode"),
            balance_after_transaction=_parse_decimal_nullable(d.get("balanceAfter")),
            balance_after_currency=d.get("balanceAfterCurrency"),
        )

    # -- Lifecycle -------------------------------------------------------------

    def close(self) -> None:
        if self._owns_http:
            self._http.close()

    def __enter__(self) -> "OpenBankingClient":
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()
