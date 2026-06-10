/// Errors raised by the open-banking.io client.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// A required configuration value was missing or blank.
    #[error("{0}")]
    Config(String),

    /// The envelope was malformed or used an unsupported version.
    #[error("invalid or unsupported envelope")]
    InvalidEnvelope,

    /// A cryptographic step (key load, ECDH, HKDF, AES-GCM) failed.
    #[error("crypto error: {0}")]
    Crypto(String),

    /// An HTTP request failed or returned a non-2xx status.
    #[error("http error: {0}")]
    Http(String),

    /// The requested account id was not present in the accounts list.
    #[error("account {0} not found")]
    AccountNotFound(String),

    /// The account has no active session, so its uid cannot be decrypted to sync.
    #[error("account has no active session (reconnect required) -- cannot sync")]
    NoActiveSession,

    /// JSON (de)serialization failed.
    #[error("json error: {0}")]
    Json(#[from] serde_json::Error),
}

pub type Result<T> = std::result::Result<T, Error>;
