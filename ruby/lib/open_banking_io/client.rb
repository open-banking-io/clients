# frozen_string_literal: true

require "json"
require "uri"
require "net/http"
require "bigdecimal"

require_relative "envelope"
require_relative "models"

module OpenBankingIO
  # Raised when the API returns a non-success HTTP status.
  class HTTPError < StandardError
    attr_reader :status, :body

    def initialize(status, body)
      @status = status
      @body = body
      super("open-banking.io request failed with HTTP #{status}")
    end
  end

  # Server-to-server client for open-banking.io.
  #
  # Authenticates with an API key (+X-Api-Key+) and decrypts the zero-knowledge data
  # envelopes locally with the exported private key -- the service only ever returns
  # ciphertext it cannot read.
  class Client
    DEFAULT_OPEN_TIMEOUT = 15
    DEFAULT_READ_TIMEOUT = 60

    # Builds a client from a credentials-bundle JSON string or a path to a bundle file.
    def self.from_credentials(path_or_json)
      raw = if File.file?(path_or_json.to_s)
        File.read(path_or_json)
      else
        path_or_json
      end

      bundle = JSON.parse(raw)
      api_base_url = bundle["apiBaseUrl"].to_s
      api_key = bundle["apiKey"]
      raise ArgumentError, "The credentials bundle has no apiKey" if api_key.nil? || api_key.empty?

      enc_key = bundle["encryptionKey"] || {}
      private_key = enc_key["privateKey"] || enc_key["privateKeyPkcs8B64"]
      if private_key.nil? || private_key.to_s.empty?
        raise ArgumentError, "The credentials bundle has no encryption private key"
      end

      new(api_base_url: api_base_url, api_key: api_key, private_key_pkcs8: private_key)
    end

    def initialize(api_base_url:, api_key:, private_key_pkcs8:)
      raise ArgumentError, "api_base_url is required" if blank?(api_base_url)
      raise ArgumentError, "api_key is required" if blank?(api_key)
      raise ArgumentError, "private_key_pkcs8 is required" if blank?(private_key_pkcs8)

      @base_uri = URI.parse(api_base_url.to_s.sub(%r{/+\z}, "") + "/")
      @api_key = api_key
      @private_key = Envelope.load_private_key(private_key_pkcs8)
    end

    # Lists the user's accounts with all sensitive fields decrypted.
    def get_accounts
      account_wires.map { |w| map_account(w) }
    end

    # Returns a page of an account's statement, newest first, with decrypted fields.
    def get_transactions(account_id, from: nil, to: nil, limit: nil, offset: nil)
      params = {}
      params["from"] = from unless from.nil?
      params["to"] = to unless to.nil?
      params["limit"] = limit unless limit.nil?
      params["offset"] = offset unless offset.nil?

      page = get_json("api/accounts/#{account_id}/transactions", params)
      items = (page["items"] || []).map { |t| map_transaction(t) }
      TransactionPage.new(items: items, total: page["total"] || 0)
    end

    # Lists the user's bank connections.
    def get_connections
      get_json("api/connections").map do |c|
        Connection.new(
          session_id: c["sessionId"] || "",
          aspsp_name: c["aspspName"] || "",
          aspsp_country: c["aspspCountry"] || "",
          valid_until: c["validUntil"],
          status: c["status"] || "",
          account_count: c["accountCount"] || 0,
          last_synced_at: c["lastSyncedAt"],
          psu_type: c["psuType"]
        )
      end
    end

    # Triggers an online sync of one account.
    #
    # Decrypts that account's Enable Banking uid and posts it, so the service can fetch
    # fresh data without ever holding the uid in plaintext.
    def sync(account_id)
      account = account_wires.find { |a| a["id"] == account_id }
      raise ArgumentError, "Account #{account_id} not found" if account.nil?

      uid = decrypt_uid(account)
      if uid.nil?
        raise ArgumentError, "Account has no active session (reconnect required) -- cannot sync"
      end

      result = post_json("api/accounts/#{account_id}/sync", {"uid" => uid})
      SyncResult.new(
        new_transactions: result["newTransactions"] || 0,
        total_fetched: result["totalFetched"] || 0
      )
    end

    # Triggers an online sync of every account that has an active session.
    def sync_all
      items = []
      account_wires.each do |a|
        uid = decrypt_uid(a)
        items << {"accountId" => a["id"], "uid" => uid} unless uid.nil?
      end

      result = post_json("api/sync", {"items" => items})
      SyncAllResult.new(
        accounts: result["accounts"] || 0,
        new_transactions: result["newTransactions"] || 0
      )
    end

    private

    def blank?(value)
      value.nil? || value.to_s.strip.empty?
    end

    def account_wires
      get_json("api/accounts")
    end

    def decrypt_uid(account)
      payload = Envelope.decrypt_to_json(@private_key, account["uidEnc"])
      payload && payload["uid"]
    end

    def map_account(a)
      acc = Envelope.decrypt_to_json(@private_key, a["enc"]) || {}
      name = Envelope.decrypt_to_json(@private_key, a["displayNameEnc"]) || {}

      balances = (a["balances"] || []).map do |b|
        dec = Envelope.decrypt_to_json(@private_key, b["enc"]) || {}
        Balance.new(
          type: b["type"] || "",
          currency: b["currency"] || "",
          reference_date: b["referenceDate"],
          name: dec["name"],
          amount: parse_decimal(dec["amount"])
        )
      end

      Account.new(
        id: a["id"] || "",
        aspsp_name: a["aspspName"] || "",
        aspsp_country: a["aspspCountry"] || "",
        currency: a["currency"] || "",
        account_type: a["accountType"],
        bic: a["bic"],
        needs_reconnect: a["needsReconnect"] || false,
        iban: acc["iban"],
        bban: acc["bban"],
        owner_name: acc["ownerName"],
        account_name: acc["accountName"],
        product: acc["product"],
        display_name: name["displayName"],
        balances: balances
      )
    end

    def map_transaction(t)
      d = Envelope.decrypt_to_json(@private_key, t["enc"]) || {}
      Transaction.new(
        id: t["id"] || "",
        currency: t["currency"] || "",
        credit_debit_indicator: t["creditDebitIndicator"] || "",
        status: t["status"],
        booking_date: t["bookingDate"],
        value_date: t["valueDate"],
        transaction_date: t["transactionDate"],
        bank_transaction_code: t["bankTransactionCode"],
        amount: parse_decimal(d["amount"]),
        creditor_name: d["creditorName"],
        creditor_iban: d["creditorIban"],
        creditor_bban: d["creditorBban"],
        creditor_agent_bic: d["creditorAgentBic"],
        debtor_name: d["debtorName"],
        debtor_iban: d["debtorIban"],
        debtor_bban: d["debtorBban"],
        debtor_agent_bic: d["debtorAgentBic"],
        remittance_information: d["remittanceInformation"],
        note: d["note"],
        reference_number: d["referenceNumber"],
        exchange_rate: d["exchangeRate"],
        merchant_category_code: d["merchantCategoryCode"],
        balance_after_transaction: parse_decimal_nullable(d["balanceAfter"]),
        balance_after_currency: d["balanceAfterCurrency"]
      )
    end

    def parse_decimal(value)
      return BigDecimal(0) if value.nil? || value == ""

      BigDecimal(value.to_s)
    end

    def parse_decimal_nullable(value)
      return nil if value.nil? || value == ""

      BigDecimal(value.to_s)
    end

    # -- HTTP ------------------------------------------------------------------

    def get_json(path, params = {})
      uri = resolve(path)
      unless params.empty?
        uri.query = URI.encode_www_form(params)
      end

      # `path` is an internal, library-controlled API route resolved against the configured
      # base URI (see #resolve), not user-supplied file/URL input. This is an HTTP API client;
      # issuing the request is its purpose.
      # nosemgrep: ruby.rails.security.audit.avoid-tainted-http-request.avoid-tainted-http-request
      request = Net::HTTP::Get.new(uri)
      send_request(uri, request)
    end

    def post_json(path, body)
      uri = resolve(path)
      request = Net::HTTP::Post.new(uri)
      request["Content-Type"] = "application/json"
      request.body = JSON.generate(body)
      send_request(uri, request)
    end

    def resolve(path)
      (@base_uri + path.sub(%r{\A/+}, "")).dup
    end

    def send_request(uri, request)
      request["X-Api-Key"] = @api_key
      request["Accept"] = "application/json"

      http = Net::HTTP.new(uri.host, uri.port)
      http.use_ssl = (uri.scheme == "https")
      http.open_timeout = DEFAULT_OPEN_TIMEOUT
      http.read_timeout = DEFAULT_READ_TIMEOUT

      response = http.request(request)
      code = response.code.to_i
      raise HTTPError.new(code, response.body) unless code.between?(200, 299)

      body = response.body
      return nil if body.nil? || body.empty?

      JSON.parse(body)
    end
  end
end
