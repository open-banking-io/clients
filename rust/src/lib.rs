//! open-banking.io Rust client.
//!
//! Server-to-server client that authenticates with an API key and decrypts the zero-knowledge data
//! envelopes locally with your exported private key -- the service only ever returns ciphertext it
//! cannot read.
//!
//! ```no_run
//! use open_banking_io::OpenBankingClient;
//!
//! # fn main() -> Result<(), Box<dyn std::error::Error>> {
//! let client = OpenBankingClient::from_credentials("credentials.json")?;
//! for account in client.get_accounts()? {
//!     let booked = account.balances.iter().find(|b| b.type_ == "ITBD");
//!     println!("{:?}: {:?} {}", account.iban, booked.map(|b| &b.amount), account.currency);
//! }
//! # Ok(())
//! # }
//! ```

mod client;
pub mod envelope;
mod error;
mod models;

pub use client::OpenBankingClient;
pub use error::{Error, Result};
pub use models::{
    Account, Balance, Connection, CredentialsBundle, EncryptionKey, SyncAllResult, SyncResult,
    Transaction, TransactionPage, TransactionQuery,
};
