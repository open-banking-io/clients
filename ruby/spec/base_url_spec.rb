# frozen_string_literal: true

require "spec_helper"
require "support/mock_server"

RSpec.describe "api_base_url normalization" do
  let(:credentials) { load_fixture("credentials.json") }
  let(:api_key) { MockServer::API_KEY }

  def build(base_url)
    OpenBankingIO::Client.new(
      api_base_url: base_url,
      api_key: credentials["apiKey"],
      private_key_pkcs8: credentials["encryptionKey"]["privateKey"]
    )
  end

  describe "whitespace" do
    it "is stripped so the request still reaches the server" do
      server = MockServer.new(FIXTURES_DIR)
      ["  #{server.base_url}", "#{server.base_url}  ", "\t#{server.base_url}\n", "  #{server.base_url}/  "].each do |padded|
        expect { build(padded).get_accounts }.not_to raise_error
      end
    ensure
      server&.stop
    end
  end

  describe "scheme" do
    %w[open-banking.io //open-banking.io ftp://open-banking.io].each do |bad|
      it "rejects #{bad.inspect}" do
        expect { build(bad) }.to raise_error(ArgumentError, /http/i)
      end
    end
  end

  describe "cleartext http" do
    ["http://open-banking.io", "http://192.168.1.10:8080", "http://localhost.evil.test"].each do |bad|
      it "rejects #{bad.inspect}" do
        expect { build(bad) }.to raise_error(ArgumentError, /https/i)
      end
    end

    ["http://localhost:8080", "http://127.0.0.1:8080", "http://[::1]:8080"].each do |ok|
      it "allows loopback #{ok.inspect}" do
        expect { build(ok) }.not_to raise_error
      end
    end
  end

  it "rejects a blank base url" do
    expect { build("   ") }.to raise_error(ArgumentError, /required/)
  end
end
