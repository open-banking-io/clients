# frozen_string_literal: true

require "spec_helper"
require "openssl"
require "base64"

# Negative-path tests for envelope parsing and key loading.
#
# These reuse the shared fixtures (a valid keypair + a valid envelope) and corrupt them in
# specific ways to assert the library rejects malformed input rather than mis-decrypting it.
RSpec.describe OpenBankingIO::Envelope do
  let(:keypair) { load_fixture("keypair.json") }
  let(:envelopes) { load_fixture("envelopes.json") }
  let(:priv) { described_class.load_private_key(keypair["privateKeyPkcs8B64"]) }

  # Raw bytes of a known-good envelope, decoded from the shared fixture.
  let(:valid_bytes) { Base64.decode64(envelopes["account"]) }

  describe "malformed envelopes" do
    it "raises on a truncated envelope (too short for header)" do
      truncated = valid_bytes.byteslice(0, 10)
      expect { described_class.decrypt(priv, truncated) }
        .to raise_error(ArgumentError, /Invalid or unsupported envelope/)
    end

    it "raises on an empty envelope" do
      expect { described_class.decrypt(priv, "".b) }
        .to raise_error(ArgumentError, /Invalid or unsupported envelope/)
    end

    it "raises on a wrong version byte" do
      tampered = valid_bytes.dup
      tampered.setbyte(0, 0x02) # only 0x01 is supported
      expect { described_class.decrypt(priv, tampered) }
        .to raise_error(ArgumentError, /Invalid or unsupported envelope/)
    end

    it "raises on a garbage ephemeral public key" do
      tampered = valid_bytes.dup
      # Clobber the 65-byte EC point (offset 1) so it is no longer a valid P-256 point.
      (1...(1 + OpenBankingIO::Envelope::POINT_LEN)).each { |i| tampered.setbyte(i, 0xFF) }
      expect { described_class.decrypt(priv, tampered) }
        .to raise_error(ArgumentError, /Invalid ephemeral public key/)
    end

    it "raises when the GCM auth tag fails (flipped ciphertext bit)" do
      tampered = valid_bytes.dup
      last = tampered.bytesize - 1
      tampered.setbyte(last, tampered.getbyte(last) ^ 0x01)
      expect { described_class.decrypt(priv, tampered) }.to raise_error(OpenSSL::Cipher::CipherError)
    end
  end

  describe "invalid private keys" do
    it "raises on a non-PKCS#8 / garbage key blob" do
      garbage = Base64.strict_encode64("not a real key at all")
      expect { described_class.load_private_key(garbage) }.to raise_error(StandardError)
    end

    it "raises when the key is a valid PKCS#8 key but not an EC key (RSA)" do
      rsa_der = OpenSSL::PKey::RSA.new(2048).private_to_der
      rsa_b64 = Base64.strict_encode64(rsa_der)
      expect { described_class.load_private_key(rsa_b64) }
        .to raise_error(ArgumentError, /not an EC key/)
    end
  end

  describe "decrypt_to_json" do
    it "raises on garbage base64 that decodes to a too-short envelope" do
      expect { described_class.decrypt_to_json(priv, "AAAA") }
        .to raise_error(ArgumentError, /Invalid or unsupported envelope/)
    end
  end
end
