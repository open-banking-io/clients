# frozen_string_literal: true

require "spec_helper"
require "support/mock_server"

# Minimal Net::HTTP-shaped stub: records the requests it is asked to send and replies
# with a canned response, so an injected client never opens a real socket.
class FakeHttp
  Response = Struct.new(:code, :body)

  attr_reader :requests

  def initialize(body)
    @body = body
    @requests = []
  end

  def request(request)
    @requests << request
    Response.new("200", @body)
  end
end

# Verifies the append-only HTTP options: configurable open/read timeouts and an
# injectable, caller-supplied http_client.
RSpec.describe OpenBankingIO::Client do
  let(:credentials) { load_fixture("credentials.json") }
  let(:private_key) { credentials["encryptionKey"]["privateKey"] }
  let(:api_key) { credentials["apiKey"] }

  describe "configurable timeouts" do
    before(:all) do
      @server = MockServer.new(FIXTURES_DIR)
    end

    after(:all) do
      @server.stop
    end

    it "applies custom open_timeout/read_timeout to the internally built Net::HTTP" do
      built = nil
      allow(Net::HTTP).to receive(:new).and_wrap_original do |orig, *args|
        built = orig.call(*args)
        built
      end

      client = described_class.new(
        api_base_url: @server.base_url,
        api_key: api_key,
        private_key_pkcs8: private_key,
        open_timeout: 3,
        read_timeout: 7
      )
      client.get_accounts

      expect(built.open_timeout).to eq(3)
      expect(built.read_timeout).to eq(7)
    end

    it "defaults to the previous timeout constants when none are given" do
      built = nil
      allow(Net::HTTP).to receive(:new).and_wrap_original do |orig, *args|
        built = orig.call(*args)
        built
      end

      client = described_class.new(
        api_base_url: @server.base_url,
        api_key: api_key,
        private_key_pkcs8: private_key
      )
      client.get_accounts

      expect(built.open_timeout).to eq(described_class::DEFAULT_OPEN_TIMEOUT)
      expect(built.read_timeout).to eq(described_class::DEFAULT_READ_TIMEOUT)
    end
  end

  describe "injectable http_client" do
    it "uses the injected client instead of building a Net::HTTP" do
      fake = FakeHttp.new(File.read(File.join(FIXTURES_DIR, "api", "accounts.json")))
      expect(Net::HTTP).not_to receive(:new)

      client = described_class.new(
        api_base_url: "https://unused.example",
        api_key: api_key,
        private_key_pkcs8: private_key,
        http_client: fake
      )
      accounts = client.get_accounts

      expect(fake.requests.length).to eq(1)
      expect(accounts.first.iban).to eq("DK6466952001724927")
    end

    it "still sends the X-Api-Key and User-Agent headers through the injected client" do
      fake = FakeHttp.new(File.read(File.join(FIXTURES_DIR, "api", "accounts.json")))

      client = described_class.new(
        api_base_url: "https://unused.example",
        api_key: api_key,
        private_key_pkcs8: private_key,
        http_client: fake
      )
      client.get_accounts

      sent = fake.requests.first
      expect(sent["x-api-key"]).to eq(api_key)
      expect(sent["user-agent"]).to start_with("open-banking-io/ruby/")
    end
  end
end
