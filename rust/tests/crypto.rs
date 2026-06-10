//! Envelope decryption (wire-format interop), verified against the shared fixtures.

use std::fs;

use open_banking_io::envelope;
use serde_json::Value;

fn fixtures() -> String {
    concat!(env!("CARGO_MANIFEST_DIR"), "/../fixtures/").to_string()
}

fn read_json(name: &str) -> Value {
    let raw = fs::read_to_string(format!("{}{}", fixtures(), name)).unwrap();
    serde_json::from_str(&raw).unwrap()
}

fn decrypt(env: &Value, key: &str) -> Value {
    let sk = envelope::load_private_key(
        read_json("keypair.json")["privateKeyPkcs8B64"]
            .as_str()
            .unwrap(),
    )
    .unwrap();
    envelope::decrypt_to::<Value>(&sk, env[key].as_str())
        .unwrap()
        .unwrap()
}

#[test]
fn decrypts_the_account_envelope() {
    let envelopes = read_json("envelopes.json");
    let expected = read_json("expected.json");
    let acc = decrypt(&envelopes, "account");
    assert_eq!(acc["iban"], expected["account"]["iban"]);
    assert_eq!(acc["ownerName"], expected["account"]["ownerName"]);
    assert_eq!(acc["bban"], expected["account"]["bban"]);
}

#[test]
fn decrypts_the_display_name_envelope() {
    let envelopes = read_json("envelopes.json");
    let expected = read_json("expected.json");
    let dn = decrypt(&envelopes, "displayName");
    assert_eq!(dn["displayName"], expected["displayName"]["displayName"]);
}

#[test]
fn decrypts_the_uid_envelope() {
    let envelopes = read_json("envelopes.json");
    let expected = read_json("expected.json");
    let uid = decrypt(&envelopes, "uid");
    assert_eq!(uid["uid"], expected["uid"]["uid"]);
}

#[test]
fn decrypts_the_balance_envelope_keeping_amount_as_string() {
    let envelopes = read_json("envelopes.json");
    let expected = read_json("expected.json");
    let bal = decrypt(&envelopes, "balance");
    assert_eq!(bal["amount"], expected["balances"]["ITBD"]["amount"]);
    assert!(bal["amount"].is_string());
    assert_eq!(bal["name"], expected["balances"]["ITBD"]["name"]);
}

#[test]
fn decrypts_the_transaction_envelope() {
    let envelopes = read_json("envelopes.json");
    let expected = read_json("expected.json");
    let tx = decrypt(&envelopes, "transaction");
    assert_eq!(tx["amount"], expected["transaction"]["amount"]);
    assert!(tx["amount"].is_string());
    assert_eq!(tx["creditorName"], expected["transaction"]["creditorName"]);
}

#[test]
fn a_wrong_key_fails_to_decrypt() {
    // A different valid P-256 PKCS#8 key derives the wrong shared secret.
    let bad = envelope::load_private_key(
        "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgdB9vjCwdrF1FckSNDHrw8M7PNNkpUB/tAK/EgAbZWvihRANCAASgg8XOKlU9VeYCee9+tQKtDSkFze10CRTA4b2gGKDlHIFw+QTf1AkjAjLfWqCJ4BctUqQtAYs0v0Y90Bw1wsBF",
    )
    .unwrap();
    let envelopes = read_json("envelopes.json");
    let res = envelope::decrypt_to::<Value>(&bad, envelopes["account"].as_str());
    assert!(res.is_err());
}
