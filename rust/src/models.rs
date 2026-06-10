//! Public, decrypted models and the internal wire DTOs.
//!
//! Monetary amounts are kept as `String` exactly as the service emits them, so no precision is
//! lost in a float/decimal round-trip; parse them with your decimal type of choice.

use serde::Deserialize;

// ---- Public, decrypted models -------------------------------------------------------------------

/// A balance snapshot. `type_` is the ISO 20022 code (ITBD booked, ITAV available, ...).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Balance {
    pub type_: String,
    pub name: Option<String>,
    pub amount: String,
    pub currency: String,
    pub reference_date: Option<String>,
}

/// A bank account with its sensitive fields decrypted.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Account {
    pub id: String,
    pub aspsp_name: String,
    pub aspsp_country: String,
    pub currency: String,
    pub account_type: Option<String>,
    pub bic: Option<String>,
    pub needs_reconnect: bool,
    pub iban: Option<String>,
    pub bban: Option<String>,
    pub owner_name: Option<String>,
    pub account_name: Option<String>,
    pub product: Option<String>,
    pub display_name: Option<String>,
    pub balances: Vec<Balance>,
}

/// A statement transaction with its sensitive fields decrypted.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Transaction {
    pub id: String,
    pub currency: String,
    pub credit_debit_indicator: String,
    pub status: Option<String>,
    pub booking_date: Option<String>,
    pub value_date: Option<String>,
    pub transaction_date: Option<String>,
    pub bank_transaction_code: Option<String>,
    pub amount: String,
    pub creditor_name: Option<String>,
    pub creditor_iban: Option<String>,
    pub creditor_bban: Option<String>,
    pub creditor_agent_bic: Option<String>,
    pub debtor_name: Option<String>,
    pub debtor_iban: Option<String>,
    pub debtor_bban: Option<String>,
    pub debtor_agent_bic: Option<String>,
    pub remittance_information: Option<String>,
    pub note: Option<String>,
    pub reference_number: Option<String>,
    pub exchange_rate: Option<String>,
    pub merchant_category_code: Option<String>,
    pub balance_after_transaction: Option<String>,
    pub balance_after_currency: Option<String>,
}

/// A page of transactions, newest first.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TransactionPage {
    pub items: Vec<Transaction>,
    pub total: u64,
}

/// A bank connection (consent).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Connection {
    pub session_id: String,
    pub aspsp_name: String,
    pub aspsp_country: String,
    pub valid_until: Option<String>,
    pub status: String,
    pub account_count: u64,
    pub last_synced_at: Option<String>,
    pub psu_type: Option<String>,
}

/// The result of syncing one account.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SyncResult {
    pub new_transactions: u64,
    pub total_fetched: u64,
}

/// The result of syncing every account with an active session.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SyncAllResult {
    pub accounts: u64,
    pub new_transactions: u64,
}

/// Query parameters for a transactions page.
#[derive(Debug, Clone, Default)]
pub struct TransactionQuery {
    pub from: Option<String>,
    pub to: Option<String>,
    pub limit: Option<u64>,
    pub offset: Option<u64>,
}

// ---- Credentials bundle -------------------------------------------------------------------------

/// The credentials bundle exported from open-banking.io (API key + encryption private key).
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct CredentialsBundle {
    pub service: String,
    pub api_base_url: String,
    pub user: String,
    pub api_key: Option<String>,
    pub encryption_key: EncryptionKey,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct EncryptionKey {
    pub scheme: String,
    pub curve: String,
    pub private_key_format: String,
    /// The PKCS#8 private key, base64-encoded.
    pub private_key: String,
    pub public_key: Option<String>,
}

// ---- Internal wire DTOs (what the API returns; sensitive fields are ciphertext) -----------------

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct AccountWire {
    pub id: String,
    pub aspsp_name: String,
    pub aspsp_country: String,
    pub currency: String,
    pub account_type: Option<String>,
    pub bic: Option<String>,
    pub needs_reconnect: bool,
    pub balances: Vec<BalanceWire>,
    pub enc: Option<String>,
    pub display_name_enc: Option<String>,
    pub uid_enc: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct BalanceWire {
    pub r#type: String,
    pub currency: String,
    pub reference_date: Option<String>,
    pub enc: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct TransactionPageWire {
    pub items: Vec<TransactionWire>,
    pub total: u64,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct TransactionWire {
    pub id: String,
    pub currency: String,
    pub credit_debit_indicator: String,
    pub status: Option<String>,
    pub booking_date: Option<String>,
    pub value_date: Option<String>,
    pub transaction_date: Option<String>,
    pub bank_transaction_code: Option<String>,
    pub enc: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct ConnectionWire {
    pub session_id: String,
    pub aspsp_name: String,
    pub aspsp_country: String,
    pub valid_until: Option<String>,
    pub status: String,
    pub account_count: u64,
    pub last_synced_at: Option<String>,
    pub psu_type: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct SyncResultWire {
    pub new_transactions: u64,
    pub total_fetched: u64,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct SyncAllResultWire {
    pub accounts: u64,
    pub new_transactions: u64,
}

// ---- Decrypted envelope payloads (the camelCase contract with the backend) ----------------------

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct AccountEnc {
    pub owner_name: Option<String>,
    pub iban: Option<String>,
    pub bban: Option<String>,
    pub account_name: Option<String>,
    pub product: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct DisplayNameEnc {
    pub display_name: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct UidEnc {
    pub uid: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct BalanceEnc {
    pub amount: Option<String>,
    pub name: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct TransactionEnc {
    pub amount: Option<String>,
    pub creditor_name: Option<String>,
    pub creditor_iban: Option<String>,
    pub creditor_bban: Option<String>,
    pub creditor_agent_bic: Option<String>,
    pub debtor_name: Option<String>,
    pub debtor_iban: Option<String>,
    pub debtor_bban: Option<String>,
    pub debtor_agent_bic: Option<String>,
    pub remittance_information: Option<String>,
    pub note: Option<String>,
    pub reference_number: Option<String>,
    pub exchange_rate: Option<String>,
    pub merchant_category_code: Option<String>,
    pub balance_after: Option<String>,
    pub balance_after_currency: Option<String>,
}
