package openbanking

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const fixturesDir = "../fixtures"

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixturesDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func readJSON(t *testing.T, name string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(readFixture(t, name), &m); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return m
}

func testPrivateKey(t *testing.T) string {
	t.Helper()
	return readJSON(t, "keypair.json")["privateKeyPkcs8B64"].(string)
}

func decryptFixture(t *testing.T, envB64 string) map[string]any {
	t.Helper()
	priv, err := loadPrivateKey(testPrivateKey(t))
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	var out map[string]any
	ok, err := decryptTo(priv, envB64, &out)
	if err != nil || !ok {
		t.Fatalf("decrypt: ok=%v err=%v", ok, err)
	}
	return out
}

func TestDecryptsAccountEnvelope(t *testing.T) {
	envelopes := readJSON(t, "envelopes.json")
	expected := readJSON(t, "expected.json")
	acc := decryptFixture(t, envelopes["account"].(string))
	exp := expected["account"].(map[string]any)
	if acc["iban"] != exp["iban"] {
		t.Errorf("iban = %v, want %v", acc["iban"], exp["iban"])
	}
	if acc["ownerName"] != exp["ownerName"] {
		t.Errorf("ownerName = %v, want %v", acc["ownerName"], exp["ownerName"])
	}
	if acc["bban"] != exp["bban"] {
		t.Errorf("bban = %v, want %v", acc["bban"], exp["bban"])
	}
}

func TestDecryptsDisplayNameEnvelope(t *testing.T) {
	envelopes := readJSON(t, "envelopes.json")
	expected := readJSON(t, "expected.json")
	dn := decryptFixture(t, envelopes["displayName"].(string))
	exp := expected["displayName"].(map[string]any)
	if dn["displayName"] != exp["displayName"] {
		t.Errorf("displayName = %v, want %v", dn["displayName"], exp["displayName"])
	}
}

func TestDecryptsUidEnvelope(t *testing.T) {
	envelopes := readJSON(t, "envelopes.json")
	expected := readJSON(t, "expected.json")
	uid := decryptFixture(t, envelopes["uid"].(string))
	exp := expected["uid"].(map[string]any)
	if uid["uid"] != exp["uid"] {
		t.Errorf("uid = %v, want %v", uid["uid"], exp["uid"])
	}
}

func TestDecryptsBalanceEnvelopeKeepingAmountAsString(t *testing.T) {
	envelopes := readJSON(t, "envelopes.json")
	expected := readJSON(t, "expected.json")
	bal := decryptFixture(t, envelopes["balance"].(string))
	itbd := expected["balances"].(map[string]any)["ITBD"].(map[string]any)
	if _, ok := bal["amount"].(string); !ok {
		t.Errorf("amount is not a string: %T", bal["amount"])
	}
	if bal["amount"] != itbd["amount"] {
		t.Errorf("amount = %v, want %v", bal["amount"], itbd["amount"])
	}
	if bal["name"] != itbd["name"] {
		t.Errorf("name = %v, want %v", bal["name"], itbd["name"])
	}
}

func TestDecryptsTransactionEnvelope(t *testing.T) {
	envelopes := readJSON(t, "envelopes.json")
	expected := readJSON(t, "expected.json")
	tx := decryptFixture(t, envelopes["transaction"].(string))
	exp := expected["transaction"].(map[string]any)
	if tx["amount"] != exp["amount"] {
		t.Errorf("amount = %v, want %v", tx["amount"], exp["amount"])
	}
	if tx["creditorName"] != exp["creditorName"] {
		t.Errorf("creditorName = %v, want %v", tx["creditorName"], exp["creditorName"])
	}
}

func TestWrongKeyFailsToDecrypt(t *testing.T) {
	// A different valid P-256 PKCS#8 key derives the wrong shared secret.
	const badKey = "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgdB9vjCwdrF1FckSNDHrw8M7PNNkpUB/tAK/EgAbZWvihRANCAASgg8XOKlU9VeYCee9+tQKtDSkFze10CRTA4b2gGKDlHIFw+QTf1AkjAjLfWqCJ4BctUqQtAYs0v0Y90Bw1wsBF"
	priv, err := loadPrivateKey(badKey)
	if err != nil {
		t.Fatalf("load bad key: %v", err)
	}
	envelopes := readJSON(t, "envelopes.json")
	var out map[string]any
	if _, err := decryptTo(priv, envelopes["account"].(string), &out); err == nil {
		t.Error("expected decryption to fail with the wrong key")
	}
}
