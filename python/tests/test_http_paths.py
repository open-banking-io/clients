"""HTTP path coverage: query building and sync edge cases (account not found, no session)."""

import json
from datetime import date

import pytest
from werkzeug import Response

from open_banking_io import OpenBankingClient

API_KEY = "obk_test_3f8b9c2e1a7d4655b0e9f2c1a8d7e6f5"
ACCOUNT_ID = "11111111-1111-4111-8111-111111111111"


def _json_response(data, status=200) -> Response:
    return Response(json.dumps(data), status=status, mimetype="application/json")


def _client(base_url, credentials):
    return OpenBankingClient(
        api_base_url=base_url,
        api_key=credentials["apiKey"],
        private_key_pkcs8=credentials["encryptionKey"]["privateKey"],
    )


# -- Query building in get_transactions --------------------------------------


def test_get_transactions_no_params_sends_empty_query(httpserver, credentials):
    captured: dict = {}

    def handler(request):
        captured["query"] = request.query_string.decode()
        return _json_response({"items": [], "total": 0})

    httpserver.expect_request(
        f"/api/accounts/{ACCOUNT_ID}/transactions", method="GET"
    ).respond_with_handler(handler)

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        page = client.get_transactions(ACCOUNT_ID)

    assert page.items == []
    assert page.total == 0
    assert captured["query"] == ""


def test_get_transactions_all_params_built(httpserver, credentials):
    captured: dict = {}

    def handler(request):
        captured["args"] = dict(request.args)
        return _json_response({"items": [], "total": 0})

    httpserver.expect_request(
        f"/api/accounts/{ACCOUNT_ID}/transactions", method="GET"
    ).respond_with_handler(handler)

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        client.get_transactions(
            ACCOUNT_ID,
            date_from=date(2026, 1, 1),
            date_to="2026-06-01",
            limit=25,
            offset=10,
        )

    args = captured["args"]
    assert args["from"] == "2026-01-01"  # date -> isoformat
    assert args["to"] == "2026-06-01"  # str passes through
    assert args["limit"] == "25"
    assert args["offset"] == "10"


# -- sync / sync_all edge cases ----------------------------------------------


def _wire_account(account_id, uid_enc):
    """Minimal account wire; only `id` and `uidEnc` matter for sync paths."""
    return {"id": account_id, "uidEnc": uid_enc, "balances": []}


def test_sync_account_not_found_raises(httpserver, credentials):
    httpserver.expect_request("/api/accounts", method="GET").respond_with_response(
        _json_response([_wire_account("other-id", None)])
    )
    with (
        _client(httpserver.url_for("").rstrip("/"), credentials) as client,
        pytest.raises(ValueError, match="not found"),
    ):
        client.sync(ACCOUNT_ID)


def test_sync_account_without_session_raises(httpserver, credentials):
    # Account exists but uidEnc is null -> decrypts to None -> no active session.
    httpserver.expect_request("/api/accounts", method="GET").respond_with_response(
        _json_response([_wire_account(ACCOUNT_ID, None)])
    )
    with (
        _client(httpserver.url_for("").rstrip("/"), credentials) as client,
        pytest.raises(ValueError, match="no active session"),
    ):
        client.sync(ACCOUNT_ID)


def test_sync_all_skips_sessionless_accounts(httpserver, credentials, envelopes):
    captured: dict = {}

    # Two accounts: one with a valid uid envelope, one with null uidEnc (skipped).
    wires = [
        _wire_account(ACCOUNT_ID, envelopes["uid"]),
        _wire_account("22222222-2222-4222-8222-222222222222", None),
    ]
    httpserver.expect_request("/api/accounts", method="GET").respond_with_response(
        _json_response(wires)
    )

    def sync_all_handler(request):
        captured["body"] = json.loads(request.get_data())
        return _json_response({"accounts": 1, "newTransactions": 0})

    httpserver.expect_request("/api/sync", method="POST").respond_with_handler(sync_all_handler)

    with _client(httpserver.url_for("").rstrip("/"), credentials) as client:
        result = client.sync_all()

    assert result.accounts == 1
    items = captured["body"]["items"]
    assert len(items) == 1  # the sessionless account was skipped
    assert items[0]["accountId"] == ACCOUNT_ID
    assert items[0]["uid"] == "c5d93aa7-5e23-4da0-ba88-42b9a584492c"
