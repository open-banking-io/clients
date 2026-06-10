# frozen_string_literal: true

require "json"
require "bigdecimal"

require "open_banking_io"

# ruby/spec/ -> ruby/ -> repo root -> fixtures/
FIXTURES_DIR = File.expand_path("../../fixtures", __dir__)

def load_fixture(*parts)
  JSON.parse(File.read(File.join(FIXTURES_DIR, *parts)))
end

RSpec.configure do |config|
  config.expect_with :rspec do |c|
    c.syntax = :expect
  end
  config.disable_monkey_patching!
end
