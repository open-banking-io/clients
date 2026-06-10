# frozen_string_literal: true

require "simplecov"
require "simplecov-cobertura"

SimpleCov.start do
  add_filter "/spec/"
  # Emit both the HTML report and a Cobertura XML at ruby/coverage/coverage.xml.
  formatter SimpleCov::Formatter::MultiFormatter.new(
    [
      SimpleCov::Formatter::HTMLFormatter,
      SimpleCov::Formatter::CoberturaFormatter
    ]
  )
end

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
