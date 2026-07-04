"""Unit tests for the open-banking.io Beancount importer.

These use a fake in-memory SDK client, so they need no credentials or network.
"""

import datetime
from dataclasses import dataclass, field
from decimal import Decimal

import pytest
from beancount.core import data

from beancount_openbanking_io import OpenBankingImporter

# --- fakes mirroring the open-banking-io SDK surface ------------------------


@dataclass
class FakeBalance:
    type: str
    amount: Decimal
    currency: str


@dataclass
class FakeTxn:
    id: str
    credit_debit_indicator: str
    amount: Decimal
    currency: str = "DKK"
    status: str | None = "BOOK"
    booking_date: datetime.date | None = datetime.date(2026, 7, 2)
    value_date: datetime.date | None = None
    transaction_date: datetime.date | None = None
    creditor_name: str | None = None
    debtor_name: str | None = None
    remittance_information: str | None = None
    note: str | None = None


@dataclass
class FakeAccount:
    id: str
    balances: list = field(default_factory=list)


@dataclass
class FakePage:
    items: list
    total: int


_UNSET = object()


class FakeClient:
    def __init__(self, txns, accounts=None, reported_total=_UNSET):
        self._txns = txns
        self._accounts = accounts or []
        self._reported_total = reported_total

    def get_transactions(self, account_id, limit, offset):
        page = self._txns[offset : offset + limit]
        total = len(self._txns) if self._reported_total is _UNSET else self._reported_total
        return FakePage(items=page, total=total)

    def get_accounts(self):
        return self._accounts


def _write_config(tmp_path, assert_balance=False):
    cfg = tmp_path / "test.openbanking.yaml"
    extra = "    assert_balance: true\n" if assert_balance else ""
    cfg.write_text(
        "credentials: ./creds.json\n"
        "accounts:\n"
        "  - id: acc-1\n"
        "    account: Assets:Bank:Nordea\n"
        "    currency: DKK\n" + extra
    )
    return str(cfg)


# --- tests ------------------------------------------------------------------


def test_identify():
    imp = OpenBankingImporter(client=FakeClient([]))
    assert imp.identify("/x/my.openbanking.yaml") is True
    assert imp.identify("/x/statement.csv") is False


def test_sign_mapping_dbit_negative_crdt_positive(tmp_path):
    txns = [
        FakeTxn("t1", "DBIT", Decimal("194.23"), creditor_name="Netto"),
        FakeTxn("t2", "CRDT", Decimal("2500.00"), debtor_name="Employer"),
    ]
    imp = OpenBankingImporter(client=FakeClient(txns))
    entries = imp.extract(_write_config(tmp_path), [])
    txn_entries = [e for e in entries if isinstance(e, data.Transaction)]
    assert len(txn_entries) == 2
    by_id = {e.meta["obio-id"]: e for e in txn_entries}
    assert by_id["t1"].postings[0].units.number == Decimal("-194.23")  # DBIT
    assert by_id["t1"].payee == "Netto"
    assert by_id["t2"].postings[0].units.number == Decimal("2500.00")  # CRDT
    assert by_id["t2"].payee == "Employer"


def test_transaction_balances_via_counterpart(tmp_path):
    txns = [FakeTxn("t1", "DBIT", Decimal("10.00"))]
    imp = OpenBankingImporter(client=FakeClient(txns))
    txn = next(
        e for e in imp.extract(_write_config(tmp_path), []) if isinstance(e, data.Transaction)
    )
    assert len(txn.postings) == 2
    assert txn.postings[1].account == "Expenses:Uncategorized"
    assert txn.postings[1].units is None  # elided -> Beancount auto-balances


def test_skips_unknown_indicator(tmp_path):
    # An unsignable transaction must be skipped, never imported with a guessed sign.
    txns = [FakeTxn("t1", "", Decimal("10.00")), FakeTxn("t2", "CRDT", Decimal("5.00"))]
    imp = OpenBankingImporter(client=FakeClient(txns))
    entries = imp.extract(_write_config(tmp_path), [])
    ids = {e.meta["obio-id"] for e in entries if isinstance(e, data.Transaction)}
    assert ids == {"t2"}


def test_skips_dateless_transaction(tmp_path):
    # A record with no booking/value/transaction date can't anchor an entry (and
    # would break the date sort) — it must be skipped, not imported or crash.
    txns = [
        FakeTxn(
            "t1",
            "CRDT",
            Decimal("5.00"),
            booking_date=None,
            value_date=None,
            transaction_date=None,
        ),
        FakeTxn("t2", "DBIT", Decimal("3.00")),
    ]
    imp = OpenBankingImporter(client=FakeClient(txns))
    entries = imp.extract(_write_config(tmp_path), [])
    ids = {e.meta["obio-id"] for e in entries if isinstance(e, data.Transaction)}
    assert ids == {"t2"}


def test_pagination_when_total_is_falsy(tmp_path):
    # If the API reports total=0 (or None) alongside full pages, we must still
    # paginate to the end via short/empty-page detection, not truncate after page 1.
    txns = [FakeTxn(f"t{i}", "CRDT", Decimal("1.00")) for i in range(1100)]
    imp = OpenBankingImporter(client=FakeClient(txns, reported_total=0))
    entries = imp.extract(_write_config(tmp_path), [])
    assert len([e for e in entries if isinstance(e, data.Transaction)]) == 1100


def test_pending_is_flagged(tmp_path):
    txns = [FakeTxn("t1", "DBIT", Decimal("9.00"), status="PDNG")]
    imp = OpenBankingImporter(client=FakeClient(txns))
    entries = imp.extract(_write_config(tmp_path), [])
    txn = next(e for e in entries if isinstance(e, data.Transaction))
    assert txn.flag == "!"


def test_dedup_against_existing_ledger(tmp_path):
    txns = [FakeTxn("t1", "DBIT", Decimal("1.00")), FakeTxn("t2", "CRDT", Decimal("2.00"))]
    imp = OpenBankingImporter(client=FakeClient(txns))
    cfg = _write_config(tmp_path)
    first = imp.extract(cfg, [])
    # Re-import with the first batch as "existing" -> nothing new.
    second = imp.extract(cfg, first)
    assert [e for e in second if isinstance(e, data.Transaction)] == []


def test_pagination(tmp_path):
    txns = [FakeTxn(f"t{i}", "CRDT", Decimal("1.00")) for i in range(1200)]
    imp = OpenBankingImporter(client=FakeClient(txns))
    entries = imp.extract(_write_config(tmp_path), [])
    assert len([e for e in entries if isinstance(e, data.Transaction)]) == 1200


def test_balance_assertion_from_booked(tmp_path):
    txns = [FakeTxn("t1", "CRDT", Decimal("5.00"))]
    accts = [
        FakeAccount(
            "acc-1",
            balances=[
                FakeBalance("ITAV", Decimal("123.60"), "DKK"),
                FakeBalance("ITBD", Decimal("111.50"), "DKK"),
            ],
        )
    ]
    imp = OpenBankingImporter(client=FakeClient(txns, accounts=accts))
    entries = imp.extract(_write_config(tmp_path, assert_balance=True), [])
    bals = [e for e in entries if isinstance(e, data.Balance)]
    assert len(bals) == 1
    assert bals[0].amount.number == Decimal("111.50")  # ITBD, not ITAV
    assert bals[0].account == "Assets:Bank:Nordea"


if __name__ == "__main__":
    raise SystemExit(pytest.main([__file__, "-v"]))
