# frozen_string_literal: true

require "spec_helper"

# Wire-format interop: decrypt the shared fixtures and assert they match expected.
RSpec.describe OpenBankingIO::Envelope do
  let(:keypair) { load_fixture("keypair.json") }
  let(:envelopes) { load_fixture("envelopes.json") }
  let(:expected) { load_fixture("expected.json") }
  let(:priv) { described_class.load_private_key(keypair["privateKeyPkcs8B64"]) }

  it "decrypts the account envelope" do
    acc = described_class.decrypt_to_json(priv, envelopes["account"])
    expect(acc).to eq(expected["account"])
    expect(acc["iban"]).to eq("DK6466952001724927")
  end

  it "decrypts the display name envelope" do
    name = described_class.decrypt_to_json(priv, envelopes["displayName"])
    expect(name).to eq(expected["displayName"])
    expect(name["displayName"]).to eq("Drift")
  end

  it "decrypts the uid envelope" do
    uid = described_class.decrypt_to_json(priv, envelopes["uid"])
    expect(uid["uid"]).to eq(expected["uid"]["uid"])
    expect(uid["uid"]).to eq("c5d93aa7-5e23-4da0-ba88-42b9a584492c")
  end

  it "decrypts the balance envelope (the booked ITBD balance)" do
    bal = described_class.decrypt_to_json(priv, envelopes["balance"])
    expect(BigDecimal(bal["amount"])).to eq(BigDecimal(expected["balances"]["ITBD"]["amount"]))
    expect(BigDecimal(bal["amount"])).to eq(BigDecimal("828.13"))
    expect(bal["name"]).to eq("Tatic")
  end

  it "decrypts the transaction envelope" do
    txn = described_class.decrypt_to_json(priv, envelopes["transaction"])
    exp = expected["transaction"]
    expect(BigDecimal(txn["amount"])).to eq(BigDecimal(exp["amount"]))
    expect(BigDecimal(txn["amount"])).to eq(BigDecimal("194.23"))
    expect(txn["creditorName"]).to eq("One.com")
    expect(txn["merchantCategoryCode"]).to eq("4816")
    expect(BigDecimal(txn["balanceAfter"])).to eq(BigDecimal("633.90"))
  end

  it "returns nil for a nil envelope" do
    expect(described_class.decrypt_to_json(priv, nil)).to be_nil
  end
end
