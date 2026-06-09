"""Public, decrypted models for the open-banking.io client."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import date, datetime
from decimal import Decimal


@dataclass
class Balance:
    """A balance snapshot. ``type`` is the ISO 20022 code (ITBD booked, ITAV available, ...)."""

    type: str
    name: str | None
    amount: Decimal
    currency: str
    reference_date: date | None


@dataclass
class Account:
    """A bank account with its sensitive fields decrypted."""

    id: str
    aspsp_name: str
    aspsp_country: str
    currency: str
    account_type: str | None
    bic: str | None
    needs_reconnect: bool
    iban: str | None
    bban: str | None
    owner_name: str | None
    account_name: str | None
    product: str | None
    display_name: str | None
    balances: list[Balance] = field(default_factory=list)


@dataclass
class Transaction:
    """A statement transaction with its sensitive fields decrypted."""

    id: str
    currency: str
    credit_debit_indicator: str
    status: str | None
    booking_date: date | None
    value_date: date | None
    transaction_date: date | None
    bank_transaction_code: str | None
    amount: Decimal
    creditor_name: str | None
    creditor_iban: str | None
    creditor_bban: str | None
    creditor_agent_bic: str | None
    debtor_name: str | None
    debtor_iban: str | None
    debtor_bban: str | None
    debtor_agent_bic: str | None
    remittance_information: str | None
    note: str | None
    reference_number: str | None
    exchange_rate: str | None
    merchant_category_code: str | None
    balance_after_transaction: Decimal | None
    balance_after_currency: str | None


@dataclass
class TransactionPage:
    """A page of transactions, newest first."""

    items: list[Transaction]
    total: int


@dataclass
class Connection:
    """A bank connection (consent)."""

    session_id: str
    aspsp_name: str
    aspsp_country: str
    valid_until: datetime | None
    status: str
    account_count: int
    last_synced_at: datetime | None
    psu_type: str | None


@dataclass
class SyncResult:
    new_transactions: int
    total_fetched: int


@dataclass
class SyncAllResult:
    accounts: int
    new_transactions: int
