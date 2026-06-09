"""Wire-format interop: decrypt the shared fixtures and assert they match expected."""

from decimal import Decimal

from open_banking_io import envelope


def _priv(keypair):
    return envelope.load_private_key(keypair["privateKeyPkcs8B64"])


def test_account_envelope(keypair, envelopes, expected):
    priv = _priv(keypair)
    acc = envelope.decrypt_to_json(priv, envelopes["account"])
    assert acc == expected["account"]
    assert acc["iban"] == "DK6466952001724927"


def test_display_name_envelope(keypair, envelopes, expected):
    priv = _priv(keypair)
    name = envelope.decrypt_to_json(priv, envelopes["displayName"])
    assert name == expected["displayName"]
    assert name["displayName"] == "Drift"


def test_uid_envelope(keypair, envelopes, expected):
    priv = _priv(keypair)
    uid = envelope.decrypt_to_json(priv, envelopes["uid"])
    assert uid["uid"] == expected["uid"]["uid"]
    assert uid["uid"] == "c5d93aa7-5e23-4da0-ba88-42b9a584492c"


def test_balance_envelope(keypair, envelopes, expected):
    priv = _priv(keypair)
    bal = envelope.decrypt_to_json(priv, envelopes["balance"])
    # The shared balance envelope is the booked (ITBD) balance.
    assert Decimal(bal["amount"]) == Decimal(expected["balances"]["ITBD"]["amount"])
    assert Decimal(bal["amount"]) == Decimal("828.13")
    assert bal["name"] == "Tatic"


def test_transaction_envelope(keypair, envelopes, expected):
    priv = _priv(keypair)
    txn = envelope.decrypt_to_json(priv, envelopes["transaction"])
    exp = expected["transaction"]
    assert Decimal(txn["amount"]) == Decimal(exp["amount"])
    assert Decimal(txn["amount"]) == Decimal("194.23")
    assert txn["creditorName"] == "One.com"
    assert txn["merchantCategoryCode"] == "4816"
    assert Decimal(txn["balanceAfter"]) == Decimal("633.90")
