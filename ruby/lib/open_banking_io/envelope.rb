# frozen_string_literal: true

require "openssl"
require "base64"
require "json"

module OpenBankingIO
  # Decrypts open-banking.io's zero-knowledge data envelopes.
  #
  # Scheme: ephemeral ECDH on NIST P-256 -> HKDF-SHA256 -> AES-256-GCM.
  # Wire: +version(1)=0x01 | ephemeralPublicKeyRaw(65) | nonce(12) | tag(16) | ciphertext+.
  # Only the user's private key can decrypt -- the service stores ciphertext it cannot read.
  module Envelope
    VERSION_BYTE = 0x01
    POINT_LEN = 65
    NONCE_LEN = 12
    TAG_LEN = 16
    HKDF_SALT = ("\x00".b * 32).freeze
    HKDF_INFO = "bank.core.ci/zk/v1".b.freeze
    GROUP = OpenSSL::PKey::EC::Group.new("prime256v1")

    module_function

    # Loads a base64 PKCS#8 EC (P-256) private key.
    def load_private_key(private_key_pkcs8_b64)
      key = OpenSSL::PKey.read(Base64.decode64(private_key_pkcs8_b64))
      unless key.is_a?(OpenSSL::PKey::EC)
        raise ArgumentError, "Private key is not an EC key"
      end

      key
    end

    # Decrypts the raw bytes of a zero-knowledge envelope, returning the plaintext bytes.
    def decrypt(private_key, envelope_bytes)
      min_len = 1 + POINT_LEN + NONCE_LEN + TAG_LEN
      if envelope_bytes.bytesize < min_len || envelope_bytes.getbyte(0) != VERSION_BYTE
        raise ArgumentError, "Invalid or unsupported envelope"
      end

      eph_pub_bytes = envelope_bytes.byteslice(1, POINT_LEN)
      nonce = envelope_bytes.byteslice(1 + POINT_LEN, NONCE_LEN)
      tag = envelope_bytes.byteslice(1 + POINT_LEN + NONCE_LEN, TAG_LEN)
      ciphertext = envelope_bytes.byteslice((1 + POINT_LEN + NONCE_LEN + TAG_LEN)..) || "".b

      pub = OpenSSL::PKey::EC::Point.new(GROUP, OpenSSL::BN.new(eph_pub_bytes, 2))
      shared = private_key.dh_compute_key(pub)

      key = OpenSSL::KDF.hkdf(
        shared,
        salt: HKDF_SALT,
        info: HKDF_INFO,
        length: 32,
        hash: "SHA256"
      )

      cipher = OpenSSL::Cipher.new("aes-256-gcm")
      cipher.decrypt
      cipher.key = key
      cipher.iv = nonce
      cipher.auth_tag = tag
      cipher.auth_data = ""
      cipher.update(ciphertext) + cipher.final
    end

    # Decrypts a base64 envelope and parses its JSON payload. +nil+ in -> +nil+ out.
    def decrypt_to_json(private_key, envelope_b64)
      return nil if envelope_b64.nil?

      plaintext = decrypt(private_key, Base64.decode64(envelope_b64))
      JSON.parse(plaintext)
    end
  end
end
