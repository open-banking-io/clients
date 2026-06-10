# frozen_string_literal: true

require "spec_helper"
require "support/mock_server"

# Integration test: serve the shared API fixtures over HTTP and exercise the client.
RSpec.describe OpenBankingIO::Client do
  ACCOUNT_ID = "11111111-1111-4111-8111-111111111111"

  let(:credentials) { load_fixture("credentials.json") }
  let(:private_key) { credentials["encryptionKey"]["privateKey"] }

  before(:all) do
    @server = MockServer.new(FIXTURES_DIR)
  end

  after(:all) do
    @server.stop
  end

  def build_client(api_key: credentials["apiKey"], key: private_key)
    described_class.new(
      api_base_url: @server.base_url,
      api_key: api_key,
      private_key_pkcs8: key
    )
  end

  it "decrypts accounts (iban, owner, display name and balances)" do
    accounts = build_client.get_accounts
    expect(accounts.length).to eq(1)

    acc = accounts.first
    expect(acc.iban).to eq("DK6466952001724927")
    expect(acc.owner_name).to eq("Tatic ApS")
    expect(acc.display_name).to eq("Drift")
    expect(acc.aspsp_name).to eq("Lunar")

    by_type = acc.balances.each_with_object({}) { |b, h| h[b.type] = b }
    expect(by_type["ITBD"].amount).to eq(BigDecimal("828.13"))
    expect(by_type["ITAV"].amount).to eq(BigDecimal("633.90"))
  end

  it "decrypts a page of transactions" do
    page = build_client.get_transactions(ACCOUNT_ID, limit: 50)
    expect(page.total).to eq(1)

    txn = page.items.first
    expect(txn.amount).to eq(BigDecimal("194.23"))
    expect(txn.creditor_name).to eq("One.com")
    expect(txn.merchant_category_code).to eq("4816")
    expect(txn.balance_after_transaction).to eq(BigDecimal("633.90"))
    expect(txn.credit_debit_indicator).to eq("DBIT")
  end

  it "lists connections" do
    connections = build_client.get_connections
    expect(connections.length).to eq(1)

    conn = connections.first
    expect(conn.session_id).to eq("22222222-2222-4222-8222-222222222222")
    expect(conn.aspsp_name).to eq("Lunar")
    expect(conn.account_count).to eq(1)
    expect(conn.psu_type).to eq("business")
  end

  it "posts the decrypted uid when syncing one account" do
    result = build_client.sync(ACCOUNT_ID)
    expect(result.total_fetched).to eq(1)
    expect(@server.captured[:sync_body]).to eq("uid" => "c5d93aa7-5e23-4da0-ba88-42b9a584492c")
  end

  it "posts the decrypted uids when syncing all accounts" do
    result = build_client.sync_all
    expect(result.accounts).to eq(1)

    item = @server.captured[:sync_all_body]["items"].first
    expect(item["uid"]).to eq("c5d93aa7-5e23-4da0-ba88-42b9a584492c")
    expect(item["accountId"]).to eq(ACCOUNT_ID)
  end

  it "raises on a wrong API key" do
    client = build_client(api_key: "wrong-key")
    expect { client.get_accounts }.to raise_error(OpenBankingIO::HTTPError)
  end

  it "raises with a structurally valid but different private key" do
    other = OpenSSL::PKey::EC.generate("prime256v1")
    wrong_key = [other.to_der].pack("m0")

    client = build_client(key: wrong_key)
    expect { client.get_accounts }.to raise_error(StandardError)
  end
end
