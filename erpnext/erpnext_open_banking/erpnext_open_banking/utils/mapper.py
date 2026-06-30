# ERPNext Open Banking © 2026
# Author: open-banking.io
# Licence: MIT


"""Maps open-banking.io decrypted transactions to ERPNext ``Bank Transaction`` fields.

The OBI API returns rich ISO 20022-style transaction data. ERPNext's
``Bank Transaction`` doctype has a simpler schema. This module bridges them,
preserving as much detail as possible while respecting ERPNext's field types.
"""

from __future__ import annotations

from decimal import Decimal
from typing import Any


def map_transaction(
    txn: dict[str, Any], bank_account: str, company: str
) -> dict[str, Any]:
    """Maps one OBI decrypted transaction to an ERPNext Bank Transaction field dict.

    ERPNext Bank Transaction key fields (from the v15+ doctype):
      - date, bank_account, company, currency
      - deposit, withdrawal  (one is set, the other is 0)
      - description, reference_number, transaction_id
      - bank_party_name, bank_party_iban, bank_party_account_number
      - status (default "Pending" — awaits reconciliation)

    The ``transaction_id`` is used as the dedup key (see connector.py).
    """
    amount = Decimal(txn.get("amount", "0"))
    is_credit = txn.get("credit_debit_indicator") == "CRDT"
    if is_credit:
        deposit = abs(amount)
        withdrawal = Decimal(0)
    else:
        deposit = Decimal(0)
        withdrawal = abs(amount)

    # Build a human-readable description from the best available text field.
    description_parts = []
    if txn.get("remittance_information"):
        description_parts.append(txn["remittance_information"])
    if txn.get("note"):
        description_parts.append(txn["note"])
    if not description_parts:
        # Fall back to counterparty name if no remittance/note
        party = txn.get("creditor_name") or txn.get("debtor_name")
        if party:
            description_parts.append(party)
    description = " — ".join(description_parts) if description_parts else "(no description)"

    # Determine counterparty: for credits the money comes FROM the debtor;
    # for debits it goes TO the creditor.
    if is_credit:
        party_name = txn.get("debtor_name")
        party_iban = txn.get("debtor_iban")
        party_account_number = txn.get("debtor_bban")
    else:
        party_name = txn.get("creditor_name")
        party_iban = txn.get("creditor_iban")
        party_account_number = txn.get("creditor_bban")

    return {
        "date": txn.get("booking_date") or txn.get("value_date"),
        "bank_account": bank_account,
        "company": company,
        "currency": txn.get("currency", ""),
        "deposit": str(deposit),
        "withdrawal": str(withdrawal),
        "description": description[:1400] if isinstance(description, str) else str(description)[:1400],
        "reference_number": (txn.get("reference_number") or "")[:140],
        "transaction_id": txn.get("id", ""),
        "bank_party_name": (party_name or "")[:140],
        "bank_party_iban": (party_iban or "")[:34],
        "bank_party_account_number": (party_account_number or "")[:34],
        "status": "Pending",
        # Extra fields stored for audit / extended bank statement section
        "transaction_type": (txn.get("bank_transaction_code") or "")[:50],
    }
