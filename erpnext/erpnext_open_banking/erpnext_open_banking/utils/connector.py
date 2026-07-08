# ERPNext Open Banking © 2026
# Author: open-banking.io
# Licence: MIT


"""Sync engine: pulls transactions from open-banking.io and writes them into ERPNext.

For each active Open Banking Connection:
  1. Fetch transactions from OBI (decrypted locally — zero-knowledge).
  2. Map each to an ERPNext ``Bank Transaction`` field dict.
  3. Dedup by ``transaction_id`` (skip if a Bank Transaction with that ID
     already exists for this bank account).
  4. Insert new Bank Transactions (status=Pending, ready for reconciliation).
  5. Log the result to an ``Open Banking Sync Log`` record.
"""

from __future__ import annotations

from datetime import date, timedelta
from typing import Any

import frappe
from frappe import _
from frappe.utils import cint, now_datetime, get_datetime
from frappe.utils.file_lock import LockTimeoutError
from frappe.utils.synchronization import filelock

from .client import OpenBankingClient
from .mapper import map_transaction


def get_client(settings: dict[str, Any] | None = None) -> OpenBankingClient:
    """Builds an OpenBankingClient from the Open Banking Settings doctype."""
    doc = frappe.get_doc("Open Banking Settings")
    if settings is None:
        settings = doc.as_dict()

    # Password field — stored encrypted, must be read via get_password().
    bundle_raw = doc.get_password("credentials_bundle", raise_exception=False)
    if not bundle_raw:
        frappe.throw(_("Please paste your open-banking.io credentials bundle in Open Banking Settings."))

    base_url_override = (settings.get("api_base_url_override") or "").strip() or None
    return OpenBankingClient.from_credentials(bundle_raw, base_url_override=base_url_override)


def sync_connection(connection_name: str) -> dict[str, Any]:
    """Syncs one Open Banking Connection (one bank account).

    Serialized per connection so an on-demand ``sync_now`` and the scheduler
    cannot interleave and double-insert the same transactions.

    Returns ``{"created": int, "skipped": int, "total": int, "errors": list}``.
    """
    try:
        with filelock(f"open_banking_sync_{connection_name}", timeout=5):
            return _sync_connection(connection_name)
    except LockTimeoutError:
        return {
            "created": 0,
            "skipped": 0,
            "total": 0,
            "errors": ["Another sync is already running for this connection"],
        }


def _sync_connection(connection_name: str) -> dict[str, Any]:
    conn = frappe.get_doc("Open Banking Connection", connection_name)
    settings = frappe.get_doc("Open Banking Settings").as_dict()

    if conn.status != "Active":
        return {"created": 0, "skipped": 0, "total": 0, "errors": ["Connection is not active"]}

    company = conn.company or settings.get("default_company")
    bank_account = conn.bank_account

    if not bank_account:
        return {"created": 0, "skipped": 0, "total": 0, "errors": ["No ERPNext Bank Account linked"]}

    # Determine date range: from last sync (or lookback days) to today.
    # The window overlaps the previous sync by a few days because banks book
    # transactions late with earlier booking dates (pending → booked); dedup by
    # transaction_id makes the re-fetch free, and a transiently failed insert
    # gets retried on later runs instead of being lost forever.
    overlap_days = 7
    lookback_days = cint(settings.get("lookback_days")) or 30
    last_synced = conn.last_synced_at
    if last_synced:
        date_from = get_datetime(last_synced).date() - timedelta(days=overlap_days)
    else:
        date_from = date.today() - timedelta(days=lookback_days)
    date_to = date.today()

    # Log record
    log = frappe.get_doc(
        {
            "doctype": "Open Banking Sync Log",
            "connection": connection_name,
            "started_at": now_datetime(),
            "status": "Running",
        }
    )
    log.insert(ignore_permissions=True)

    created = 0
    skipped = 0
    total = 0
    errors: list[str] = []
    fetch_completed = False

    client = None
    try:
        client = get_client(settings)

        # Trigger an online sync first so the OBI service fetches fresh data
        # from the bank, then pull transactions.
        try:
            client.sync(conn.obi_account_id)
        except Exception as exc:
            # Sync may fail if consent expired — we still try to pull cached data.
            errors.append(f"Online sync warning: {exc}")

        # Paginate through all transactions in the date range.
        offset = 0
        page_size = 100
        all_txns: list[dict[str, Any]] = []

        while True:
            page = client.get_transactions(
                conn.obi_account_id,
                date_from=date_from.isoformat(),
                date_to=date_to.isoformat(),
                limit=page_size,
                offset=offset,
            )
            batch = page.get("items", [])
            total = page.get("total", total)
            if not batch:
                break
            all_txns.extend(batch)
            offset += len(batch)
            if offset >= total:
                break

        fetch_completed = True

        # Batch-fetch which of the fetched transaction_ids already exist for
        # this bank account — one SELECT bounded by the fetched batch, instead
        # of scanning the account's entire transaction history.
        fetched_ids = [t.get("id") for t in all_txns if t.get("id")]
        existing_ids: set[str] = set()
        if fetched_ids:
            existing_ids = set(
                frappe.get_all(
                    "Bank Transaction",
                    filters={
                        "bank_account": bank_account,
                        "transaction_id": ("in", fetched_ids),
                    },
                    pluck="transaction_id",
                )
            )

        # Insert each transaction (dedup by transaction_id).
        for txn in all_txns:
            txn_id = txn.get("id", "")
            if not txn_id:
                continue

            if txn_id in existing_ids:
                skipped += 1
                continue

            # An undecryptable envelope (flagged by the client) must not brick
            # the sync: report it and move on. The overlap window retries it on
            # later runs in case it was transient.
            if txn.get("_decrypt_error"):
                errors.append(f"Txn {txn_id}: decrypt failed: {txn['_decrypt_error']}")
                continue

            try:
                mapped = map_transaction(txn, bank_account, company)
                if mapped["date"] is None:
                    skipped += 1
                    continue

                bt = frappe.get_doc({"doctype": "Bank Transaction", **mapped})
                bt.flags.ignore_permissions = True
                bt.insert()
                # Bank Transaction is submittable, and the Bank Reconciliation
                # Tool only picks up submitted (docstatus 1) transactions.
                bt.submit()
                created += 1
                existing_ids.add(txn_id)
            except Exception as txn_exc:
                # One bad transaction must not abort the entire sync.
                errors.append(f"Txn {txn_id}: {txn_exc}")
                frappe.log_error(
                    title=f"Open Banking: skipped transaction {txn_id}",
                    message=str(txn_exc),
                )

    except Exception as exc:
        errors.append(str(exc))
        frappe.log_error(
            title=f"Open Banking sync failed: {connection_name}",
            message=str(exc),
        )
    finally:
        if client is not None:
            client.close()

    # Only advance last_synced_at if we processed at least one transaction
    # (created or skipped). Otherwise the failed window would be silently
    # skipped on the next run.
    if created > 0 or skipped > 0:
        frappe.db.set_value("Open Banking Connection", connection_name, "last_synced_at", now_datetime())

    # Update log. A run that never completed the fetch (credentials/network
    # failure) is Failed, not merely "completed with errors".
    if not fetch_completed:
        status = "Failed"
    elif errors:
        status = "Completed with errors"
    else:
        status = "Completed"
    log.db_set(
        {
            "completed_at": now_datetime(),
            "accounts_fetched": 1 if fetch_completed else 0,
            "transactions_created": created,
            "transactions_skipped": skipped,
            "total_available": total,
            "status": status,
            "error_detail": "\n".join(errors) if errors else "",
        }
    )

    return {"created": created, "skipped": skipped, "total": total, "errors": errors}


def sync_all_connections() -> list[dict[str, Any]]:
    """Syncs every active Open Banking Connection. Called by the scheduler.

    Respects the ``enable_scheduled_sync`` checkbox in Settings — if the user
    has disabled it, the cron tick is a no-op.
    """
    settings = frappe.get_doc("Open Banking Settings").as_dict()
    if not settings.get("enable_scheduled_sync"):
        return []

    connections = frappe.get_all(
        "Open Banking Connection", filters={"status": "Active"}, pluck="name"
    )
    results = []
    for name in connections:
        try:
            result = sync_connection(name)
            results.append({"connection": name, **result})
        except Exception as exc:
            results.append({"connection": name, "error": str(exc)})
    return results


@frappe.whitelist(methods=["POST"])
def sync_now(bank_account: str | None = None) -> dict[str, Any]:
    """Whitelisted method for the 'Sync Now' button on the Bank Account form.

    Only Accounts Managers (or System Managers) may trigger a sync.
    """
    frappe.only_for(("Accounts Manager", "System Manager"))

    filters = {"status": "Active"}
    if bank_account:
        filters["bank_account"] = bank_account

    connections = frappe.get_all("Open Banking Connection", filters=filters, pluck="name")
    if not connections:
        return {"created": 0, "skipped": 0, "total": 0, "errors": ["No active connection found"]}

    results = []
    total_created = 0
    total_skipped = 0
    all_errors: list[str] = []

    for name in connections:
        r = sync_connection(name)
        results.append({"connection": name, **r})
        total_created += r.get("created", 0)
        total_skipped += r.get("skipped", 0)
        all_errors.extend(r.get("errors", []))

    return {
        "created": total_created,
        "skipped": total_skipped,
        "connections": results,
        "errors": all_errors,
    }
