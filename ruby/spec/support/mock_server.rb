# frozen_string_literal: true

require "socket"
require "json"
require "uri"

# A tiny HTTP/1.1 mock server backed by a single TCPServer thread.
#
# It serves the shared API fixtures, enforces the +X-Api-Key+ header (401 otherwise),
# and records the bodies POSTed to the sync endpoints so tests can assert on them.
class MockServer
  API_KEY = "obk_test_3f8b9c2e1a7d4655b0e9f2c1a8d7e6f5"
  ACCOUNT_ID = "11111111-1111-4111-8111-111111111111"

  attr_reader :captured

  def initialize(fixtures_dir)
    @fixtures_dir = fixtures_dir
    @captured = {}
    @server = TCPServer.new("127.0.0.1", 0)
    @port = @server.addr[1]
    @thread = Thread.new { serve_loop }
  end

  def base_url
    "http://127.0.0.1:#{@port}"
  end

  def stop
    @running = false
    @server.close
  rescue IOError
    # already closed
  ensure
    @thread&.kill
  end

  private

  def fixture(name)
    File.read(File.join(@fixtures_dir, "api", name))
  end

  def serve_loop
    @running = true
    while @running
      client = nil
      begin
        client = @server.accept
        handle(client)
      rescue IOError, Errno::EBADF
        break
      rescue
        # keep the loop alive on per-connection errors
      ensure
        client&.close
      end
    end
  end

  def handle(client)
    request_line = client.gets
    return if request_line.nil?

    method, target, = request_line.split(" ")
    headers = {}
    while (line = client.gets) && line != "\r\n" && line != "\n"
      key, value = line.split(":", 2)
      headers[key.strip.downcase] = value.to_s.strip if value
    end

    body = ""
    if (len = headers["content-length"]&.to_i) && len.positive?
      body = client.read(len)
    end

    path = URI.parse(target).path

    unless headers["x-api-key"] == API_KEY
      respond(client, 401, JSON.generate({"error" => "unauthorized"}))
      return
    end

    status, payload = route(method, path, body)
    respond(client, status, payload)
  end

  def route(method, path, body)
    case [method, path]
    when ["GET", "/api/accounts"]
      [200, fixture("accounts.json")]
    when ["GET", "/api/accounts/#{ACCOUNT_ID}/transactions"]
      [200, fixture("transactions.json")]
    when ["GET", "/api/connections"]
      [200, fixture("connections.json")]
    when ["POST", "/api/accounts/#{ACCOUNT_ID}/sync"]
      @captured[:sync_body] = JSON.parse(body)
      [200, fixture("sync.json")]
    when ["POST", "/api/sync"]
      @captured[:sync_all_body] = JSON.parse(body)
      [200, fixture("sync-all.json")]
    else
      [404, JSON.generate({"error" => "not found"})]
    end
  end

  def respond(client, status, payload)
    payload ||= ""
    reasons = {200 => "OK", 401 => "Unauthorized", 404 => "Not Found"}
    client.write("HTTP/1.1 #{status} #{reasons.fetch(status, "OK")}\r\n")
    client.write("Content-Type: application/json\r\n")
    client.write("Content-Length: #{payload.bytesize}\r\n")
    client.write("Connection: close\r\n")
    client.write("\r\n")
    client.write(payload)
  end
end
