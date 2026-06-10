//! Decrypts open-banking.io's zero-knowledge data envelopes.
//!
//! Scheme: ephemeral ECDH on NIST P-256 -> HKDF-SHA256 -> AES-256-GCM.
//! Wire: `version(1)=0x01 | ephemeralPublicKeyRaw(65) | nonce(12) | tag(16) | ciphertext`.
//! Only the user's private key can decrypt -- the service stores ciphertext it cannot read.

use aes_gcm::{aead::Aead, Aes256Gcm, KeyInit, Nonce};
use base64::{engine::general_purpose::STANDARD as BASE64, Engine};
use hkdf::Hkdf;
use p256::{ecdh::diffie_hellman, pkcs8::DecodePrivateKey, PublicKey, SecretKey};
use serde::de::DeserializeOwned;
use sha2::Sha256;
use zeroize::Zeroizing;

use crate::error::{Error, Result};

const VERSION: u8 = 0x01;
const POINT_LEN: usize = 65;
const NONCE_LEN: usize = 12;
const TAG_LEN: usize = 16;
const HEADER_LEN: usize = 1 + POINT_LEN + NONCE_LEN + TAG_LEN;
const HKDF_SALT: [u8; 32] = [0u8; 32];
const HKDF_INFO: &[u8] = b"bank.core.ci/zk/v1";

/// Loads a base64 PKCS#8 EC (SECP256R1) private key.
pub fn load_private_key(private_key_pkcs8_b64: &str) -> Result<SecretKey> {
    let der = BASE64
        .decode(private_key_pkcs8_b64.trim())
        .map_err(|e| Error::Crypto(format!("invalid base64 private key: {e}")))?;
    SecretKey::from_pkcs8_der(&der).map_err(|e| Error::Crypto(format!("invalid PKCS#8 key: {e}")))
}

/// Decrypts the raw bytes of a zero-knowledge envelope.
pub fn decrypt(private_key: &SecretKey, envelope: &[u8]) -> Result<Vec<u8>> {
    if envelope.len() < HEADER_LEN || envelope[0] != VERSION {
        return Err(Error::InvalidEnvelope);
    }

    let eph_pub_bytes = &envelope[1..1 + POINT_LEN];
    let nonce = &envelope[1 + POINT_LEN..1 + POINT_LEN + NONCE_LEN];
    let tag = &envelope[1 + POINT_LEN + NONCE_LEN..HEADER_LEN];
    let ciphertext = &envelope[HEADER_LEN..];

    let eph_pub = PublicKey::from_sec1_bytes(eph_pub_bytes)
        .map_err(|e| Error::Crypto(format!("invalid ephemeral public key: {e}")))?;
    let shared = diffie_hellman(private_key.to_nonzero_scalar(), eph_pub.as_affine());
    // Copy the ECDH secret into a buffer that is wiped on drop. (`SharedSecret` itself zeroizes,
    // but we keep the derived material under our own `Zeroizing` guards too.)
    let shared_secret = Zeroizing::new(shared.raw_secret_bytes().to_vec());

    let hk = Hkdf::<Sha256>::new(Some(&HKDF_SALT), &shared_secret);
    let mut key = Zeroizing::new([0u8; 32]);
    hk.expand(HKDF_INFO, key.as_mut())
        .map_err(|e| Error::Crypto(format!("hkdf expand failed: {e}")))?;

    // aes-gcm expects ciphertext||tag.
    let mut ct_with_tag = Vec::with_capacity(ciphertext.len() + TAG_LEN);
    ct_with_tag.extend_from_slice(ciphertext);
    ct_with_tag.extend_from_slice(tag);

    let nonce: [u8; NONCE_LEN] = nonce
        .try_into()
        .map_err(|_| Error::Crypto("invalid nonce length".into()))?;
    let cipher = Aes256Gcm::new_from_slice(key.as_ref())
        .map_err(|e| Error::Crypto(format!("invalid aes key: {e}")))?;
    cipher
        .decrypt(&Nonce::from(nonce), ct_with_tag.as_ref())
        .map_err(|_| Error::Crypto("aes-gcm decryption failed".into()))
}

/// Decrypts a base64 envelope and parses its JSON payload. `None` in -> `None`.
pub fn decrypt_to<T: DeserializeOwned>(
    private_key: &SecretKey,
    envelope_b64: Option<&str>,
) -> Result<Option<T>> {
    let Some(b64) = envelope_b64 else {
        return Ok(None);
    };
    let bytes = BASE64
        .decode(b64)
        .map_err(|e| Error::Crypto(format!("invalid base64 envelope: {e}")))?;
    let plaintext = decrypt(private_key, &bytes)?;
    Ok(Some(serde_json::from_slice(&plaintext)?))
}
