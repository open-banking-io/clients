# frozen_string_literal: true

require_relative "lib/open_banking_io/version"

Gem::Specification.new do |spec|
  spec.name = "open-banking-io"
  spec.version = OpenBankingIO::VERSION
  spec.authors = ["open-banking.io"]

  spec.summary = "Server-to-server client for open-banking.io with local zero-knowledge envelope decryption."
  spec.description = "Authenticates with your API key and decrypts open-banking.io's zero-knowledge " \
                     "data envelopes locally with your exported private key (ECDH P-256 -> HKDF-SHA256 " \
                     "-> AES-256-GCM). The service only ever returns ciphertext it cannot read."
  spec.homepage = "https://open-banking.io"
  spec.license = "MIT"
  spec.required_ruby_version = ">= 3.1"

  spec.metadata["homepage_uri"] = spec.homepage
  spec.metadata["source_code_uri"] = "https://github.com/open-banking-io/clients"
  spec.metadata["rubygems_mfa_required"] = "true"

  spec.files = Dir["lib/**/*.rb"] + ["README.md"]
  spec.require_paths = ["lib"]

  # Runtime: stdlib only (OpenSSL, JSON, net/http). bigdecimal and base64 are bundled gems
  # that stopped being default gems in Ruby 3.4, so they must be declared explicitly.
  spec.add_dependency "base64", ">= 0.1"
  spec.add_dependency "bigdecimal", ">= 3.0"

  spec.add_development_dependency "rake", "~> 13.0"
  spec.add_development_dependency "rspec", "~> 3.0"
  spec.add_development_dependency "simplecov", "~> 0.22"
  spec.add_development_dependency "simplecov-cobertura", "~> 3.1"
  spec.add_development_dependency "rexml", "~> 3.4"
  spec.add_development_dependency "standard", "~> 1.0"
end
