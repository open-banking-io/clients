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

fn good_key() -> p256::SecretKey {
    envelope::load_private_key(
        read_json("keypair.json")["privateKeyPkcs8B64"]
            .as_str()
            .unwrap(),
    )
    .unwrap()
}

#[test]
fn a_truncated_envelope_is_rejected() {
    // A valid base64 envelope decoded then sliced shorter than the fixed header.
    let envelopes = read_json("envelopes.json");
    let bytes = base64_decode(envelopes["account"].as_str().unwrap());
    let truncated = &bytes[..10]; // < HEADER_LEN (1 + 65 + 12 + 16 = 94)
    let res = envelope::decrypt(&good_key(), truncated);
    assert!(matches!(res, Err(open_banking_io::Error::InvalidEnvelope)));
}

#[test]
fn an_empty_envelope_is_rejected() {
    let res = envelope::decrypt(&good_key(), &[]);
    assert!(matches!(res, Err(open_banking_io::Error::InvalidEnvelope)));
}

#[test]
fn a_wrong_version_byte_is_rejected() {
    // Full-length envelope but the leading version byte is flipped from 0x01.
    let envelopes = read_json("envelopes.json");
    let mut bytes = base64_decode(envelopes["account"].as_str().unwrap());
    bytes[0] = 0x02;
    let res = envelope::decrypt(&good_key(), &bytes);
    assert!(matches!(res, Err(open_banking_io::Error::InvalidEnvelope)));
}

#[test]
fn an_invalid_pkcs8_key_string_is_rejected() {
    // Well-formed base64 that is not a valid PKCS#8 EC key.
    let res = envelope::load_private_key("bm90LWEta2V5");
    assert!(matches!(res, Err(open_banking_io::Error::Crypto(_))));

    // Not even valid base64.
    let res = envelope::load_private_key("!!! not base64 !!!");
    assert!(matches!(res, Err(open_banking_io::Error::Crypto(_))));
}

#[test]
fn an_off_curve_ephemeral_public_key_is_rejected() {
    // Full-length, correct version, but the 65-byte SEC1 point is garbage:
    // 0x04 (uncompressed prefix) followed by coordinates not on the P-256 curve.
    let mut bytes = vec![0u8; 1 + 65 + 12 + 16 + 4];
    bytes[0] = 0x01; // valid version
    bytes[1] = 0x04; // uncompressed point prefix
    for b in bytes.iter_mut().skip(2).take(64) {
        *b = 0xFF; // (0xFF.., 0xFF..) is not a point on secp256r1
    }
    let res = envelope::decrypt(&good_key(), &bytes);
    assert!(matches!(res, Err(open_banking_io::Error::Crypto(_))));
}

fn base64_decode(s: &str) -> Vec<u8> {
    use base64::{engine::general_purpose::STANDARD, Engine};
    STANDARD.decode(s.trim()).unwrap()
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
