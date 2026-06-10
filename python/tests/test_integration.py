"""Integration test: serve the shared API fixtures over HTTP and exercise the client."""

import json
from decimal import Decimal

import httpx
import pytest
from werkzeug import Response

from open_banking_io import OpenBankingClient

API_KEY = "obk_test_3f8b9c2e1a7d4655b0e9f2c1a8d7e6f5"


def _json_response(data, status=200) -> Response:
    return Response(json.dumps(data), status=status, mimetype="application/json")


def _unauthorized() -> Response:
    return _json_response({"error": "unauthorized"}, status=401)


@pytest.fixture
def server(httpserver, fixtures_dir):
    account_id = "11111111-1111-4111-8111-111111111111"
    captured: dict = {}

    def load(path):
        return json.loads((fixtures_dir / "api" / path).read_text())

    def static_handler(path):
        data = load(path)

        def handler(request):
            if request.headers.get("X-Api-Key") != API_KEY:
                return _unauthorized()
            captured["user_agent"] = request.headers.get("User-Agent")
            return _json_response(data)

        return handler

    httpserver.expect_request("/api/accounts", method="GET").respond_with_handler(
        static_handler("accounts.json")
    )
    httpserver.expect_request(
        f"/api/accounts/{account_id}/transactions", method="GET"
    ).respond_with_handler(static_handler("transactions.json"))
    httpserver.expect_request("/api/connections", method="GET").respond_with_handler(
        static_handler("connections.json")
    )

    sync_data = load("sync.json")

    def sync_handler(request):
        if request.headers.get("X-Api-Key") != API_KEY:
            return _unauthorized()
        captured["sync_body"] = json.loads(request.get_data())
        return _json_response(sync_data)

    httpserver.expect_request(
        f"/api/accounts/{account_id}/sync", method="POST"
    ).respond_with_handler(sync_handler)

    sync_all_data = load("sync-all.json")

    def sync_all_handler(request):
        if request.headers.get("X-Api-Key") != API_KEY:
            return _unauthorized()
        captured["sync_all_body"] = json.loads(request.get_data())
        return _json_response(sync_all_data)

    httpserver.expect_request("/api/sync", method="POST").respond_with_handler(sync_all_handler)

    httpserver._captured = captured  # type: ignore[attr-defined]
    return httpserver


def _client(server, credentials):
    return OpenBankingClient(
        api_base_url=server.url_for("").rstrip("/"),
        api_key=credentials["apiKey"],
        private_key_pkcs8=credentials["encryptionKey"]["privateKey"],
    )


def test_get_accounts_decrypts(server, credentials):
    with _client(server, credentials) as client:
        accounts = client.get_accounts()

    assert len(accounts) == 1
    acc = accounts[0]
    assert acc.iban == "DK6466952001724927"
    assert acc.owner_name == "Tatic ApS"
    assert acc.display_name == "Drift"
    assert acc.aspsp_name == "Lunar"

    by_type = {b.type: b for b in acc.balances}
    assert by_type["ITBD"].amount == Decimal("828.13")
    assert by_type["ITAV"].amount == Decimal("633.90")


def test_get_transactions_decrypts(server, credentials):
    with _client(server, credentials) as client:
        page = client.get_transactions("11111111-1111-4111-8111-111111111111", limit=50)

    assert page.total == 1
    txn = page.items[0]
    assert txn.amount == Decimal("194.23")
    assert txn.creditor_name == "One.com"
    assert txn.merchant_category_code == "4816"
    assert txn.balance_after_transaction == Decimal("633.90")
    assert txn.credit_debit_indicator == "DBIT"


def test_sends_user_agent_header(server, credentials):
    with _client(server, credentials) as client:
        client.get_accounts()

    user_agent = server._captured["user_agent"]
    assert user_agent is not None
    assert user_agent.startswith("open-banking-io/python/")


def test_get_connections(server, credentials):
    with _client(server, credentials) as client:
        connections = client.get_connections()

    assert len(connections) == 1
    conn = connections[0]
    assert conn.session_id == "22222222-2222-4222-8222-222222222222"
    assert conn.aspsp_name == "Lunar"
    assert conn.account_count == 1
    assert conn.psu_type == "business"


def test_sync_posts_decrypted_uid(server, credentials):
    with _client(server, credentials) as client:
        result = client.sync("11111111-1111-4111-8111-111111111111")

    assert result.total_fetched == 1
    body = server._captured["sync_body"]
    assert body == {"uid": "c5d93aa7-5e23-4da0-ba88-42b9a584492c"}


def test_sync_all_posts_decrypted_uids(server, credentials):
    with _client(server, credentials) as client:
        result = client.sync_all()

    assert result.accounts == 1
    body = server._captured["sync_all_body"]
    assert body["items"][0]["uid"] == "c5d93aa7-5e23-4da0-ba88-42b9a584492c"
    assert body["items"][0]["accountId"] == "11111111-1111-4111-8111-111111111111"


def test_wrong_api_key_raises(server, credentials):
    client = OpenBankingClient(
        api_base_url=server.url_for("").rstrip("/"),
        api_key="wrong-key",
        private_key_pkcs8=credentials["encryptionKey"]["privateKey"],
    )
    with pytest.raises(httpx.HTTPStatusError):
        client.get_accounts()
    client.close()


def test_wrong_private_key_raises(server, credentials):
    # A structurally valid but different EC key must fail to decrypt (GCM auth tag).
    import base64

    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric import ec

    other = ec.generate_private_key(ec.SECP256R1())
    der = other.private_bytes(
        encoding=serialization.Encoding.DER,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    )
    wrong_key = base64.b64encode(der).decode()

    client = OpenBankingClient(
        api_base_url=server.url_for("").rstrip("/"),
        api_key=credentials["apiKey"],
        private_key_pkcs8=wrong_key,
    )
    with pytest.raises(Exception):
        client.get_accounts()
    client.close()
