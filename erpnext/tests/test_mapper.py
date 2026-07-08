# ERPNext Open Banking © 2026
# Author: open-banking.io
# Licence: MIT

"""Tests for the transaction mapper.

These are pure-function tests — no Frappe, no network. They verify that
OBI decrypted transaction dicts map correctly to ERPNext Bank Transaction fields.
"""

from datetime import date
from decimal import Decimal

import pytest

from erpnext_open_banking.erpnext_open_banking.utils.mapper import map_transaction

BANK_ACCOUNT = "HDFC-001"
COMPANY = "My Company"


class TestMapTransaction:
    def test_credit_transaction(self):
        """A CRDT (incoming) transaction maps to deposit > 0, withdrawal = 0."""
        txn = {
            "id": "txn-001",
            "currency": "EUR",
            "credit_debit_indicator": "CRDT",
            "booking_date": date(2026, 6, 15),
            "amount": "1500.00",
            "debtor_name": "ACME Corp",
            "debtor_iban": "DK1234567890123456",
            "remittance_information": "Invoice #12345",
        }
        result = map_transaction(txn, BANK_ACCOUNT, COMPANY)
        assert result["deposit"] == "1500.00"
        assert result["withdrawal"] == "0"
        assert result["bank_account"] == BANK_ACCOUNT
        assert result["company"] == COMPANY
        assert result["currency"] == "EUR"
        assert result["transaction_id"] == "txn-001"
        assert result["bank_party_name"] == "ACME Corp"
        assert result["bank_party_iban"] == "DK1234567890123456"
        assert result["status"] == "Pending"
        assert "Invoice #12345" in result["description"]

    def test_debit_transaction(self):
        """A DBIT (outgoing) transaction maps to withdrawal > 0, deposit = 0."""
        txn = {
            "id": "txn-002",
            "currency": "DKK",
            "credit_debit_indicator": "DBIT",
            "booking_date": date(2026, 6, 16),
            "amount": "299.95",
            "creditor_name": "Power Company A/S",
            "creditor_iban": "DK9876543210987654",
            "remittance_information": "Electricity bill June 2026",
        }
        result = map_transaction(txn, BANK_ACCOUNT, COMPANY)
        assert result["withdrawal"] == "299.95"
        assert result["deposit"] == "0"
        assert result["bank_party_name"] == "Power Company A/S"
        assert result["bank_party_iban"] == "DK9876543210987654"

    def test_negative_amount_debit(self):
        """A negative amount with DBIT indicator maps correctly."""
        txn = {
            "id": "txn-003",
            "currency": "EUR",
            "credit_debit_indicator": "DBIT",
            "booking_date": date(2026, 6, 17),
            "amount": "-50.00",
            "creditor_name": "Coffee Shop",
        }
        result = map_transaction(txn, BANK_ACCOUNT, COMPANY)
        assert result["withdrawal"] == "50.00"
        assert result["deposit"] == "0"

    def test_fallback_description(self):
        """When no remittance/note, description falls back to counterparty name."""
        txn = {
            "id": "txn-004",
            "currency": "EUR",
            "credit_debit_indicator": "CRDT",
            "booking_date": date(2026, 6, 18),
            "amount": "100.00",
            "debtor_name": "John Doe",
        }
        result = map_transaction(txn, BANK_ACCOUNT, COMPANY)
        assert result["description"] == "John Doe"

    def test_no_description(self):
        """When no text fields at all, description is a placeholder."""
        txn = {
            "id": "txn-005",
            "currency": "EUR",
            "credit_debit_indicator": "CRDT",
            "booking_date": date(2026, 6, 19),
            "amount": "10.00",
        }
        result = map_transaction(txn, BANK_ACCOUNT, COMPANY)
        assert result["description"] == "(no description)"

    def test_long_description_truncated(self):
        """Description is truncated to fit ERPNext's Small Text field."""
        txn = {
            "id": "txn-006",
            "currency": "EUR",
            "credit_debit_indicator": "CRDT",
            "booking_date": date(2026, 6, 20),
            "amount": "1.00",
            "remittance_information": "x" * 2000,
        }
        result = map_transaction(txn, BANK_ACCOUNT, COMPANY)
        assert len(result["description"]) <= 1400

    def test_missing_booking_date_falls_back_to_value_date(self):
        """When booking_date is None, value_date is used."""
        txn = {
            "id": "txn-007",
            "currency": "EUR",
            "credit_debit_indicator": "CRDT",
            "booking_date": None,
            "value_date": date(2026, 6, 21),
            "amount": "5.00",
        }
        result = map_transaction(txn, BANK_ACCOUNT, COMPANY)
        assert result["date"] == date(2026, 6, 21)

    def test_transaction_type_from_bank_code(self):
        """bank_transaction_code populates the transaction_type field."""
        txn = {
            "id": "txn-008",
            "currency": "EUR",
            "credit_debit_indicator": "DBIT",
            "booking_date": date(2026, 6, 22),
            "amount": "20.00",
            "bank_transaction_code": "PMNT-RDDT-DMCT",
        }
        result = map_transaction(txn, BANK_ACCOUNT, COMPANY)
        assert result["transaction_type"] == "PMNT-RDDT-DMCT"
