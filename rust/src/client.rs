//! Server-to-server client for open-banking.io. Authenticates with an API key (`X-Api-Key`) and
//! decrypts the zero-knowledge data envelopes locally with the exported private key -- the service
//! only ever returns ciphertext it cannot read.

use std::fs;
use std::path::Path;
use std::time::Duration;

use p256::SecretKey;
use serde::de::DeserializeOwned;
use serde::Serialize;

use crate::envelope;
use crate::error::{Error, Result};
use crate::models::*;

/// `User-Agent` sent on every request, e.g. `open-banking-io/rust/0.1.0`.
const USER_AGENT: &str = concat!("open-banking-io/rust/", env!("CARGO_PKG_VERSION"));

/// Decrypting client for the open-banking.io API.
pub struct OpenBankingClient {
    base_url: String,
    api_key: String,
    private_key: SecretKey,
    agent: ureq::Agent,
}

impl OpenBankingClient {
    /// Builds a client from an API base url, API key, and base64 PKCS#8 encryption private key.
    pub fn new(api_base_url: &str, api_key: &str, private_key_pkcs8_b64: &str) -> Result<Self> {
        if api_base_url.trim().is_empty() {
            return Err(Error::Config("api_base_url is required".into()));
        }
        if api_key.trim().is_empty() {
            return Err(Error::Config("api_key is required".into()));
        }
        if private_key_pkcs8_b64.trim().is_empty() {
            return Err(Error::Config("private_key_pkcs8 is required".into()));
        }

        Ok(Self {
            base_url: api_base_url.trim_end_matches('/').to_string(),
            api_key: api_key.to_string(),
            private_key: envelope::load_private_key(private_key_pkcs8_b64)?,
            agent: ureq::AgentBuilder::new()
                .timeout_connect(Duration::from_secs(10))
                .timeout(Duration::from_secs(30))
                .user_agent(USER_AGENT)
                .build(),
        })
    }

    /// Builds a client from a credentials-bundle JSON string or a path to a bundle file.
    pub fn from_credentials(path_or_json: &str) -> Result<Self> {
        // `path_or_json` is a credentials source chosen by the trusted application integrator
        // (a file path *or* an inline JSON bundle), not untrusted end-user input. Accepting an
        // arbitrary local path here is the documented behaviour of this accessor.
        // nosemgrep: rust.actix.path-traversal.tainted-path.tainted-path
        let raw = if Path::new(path_or_json).exists() {
            // nosemgrep: rust.actix.path-traversal.tainted-path.tainted-path
            fs::read_to_string(path_or_json)
                .map_err(|e| Error::Config(format!("could not read credentials file: {e}")))?
        } else {
            path_or_json.to_string()
        };
        let bundle: CredentialsBundle = serde_json::from_str(&raw)?;
        Self::from_bundle(&bundle)
    }

    /// Builds a client from an already-parsed credentials bundle.
    pub fn from_bundle(bundle: &CredentialsBundle) -> Result<Self> {
        let api_key = bundle
            .api_key
            .as_deref()
            .filter(|k| !k.trim().is_empty())
            .ok_or_else(|| Error::Config("the credentials bundle has no apiKey".into()))?;
        if bundle.encryption_key.private_key.trim().is_empty() {
            return Err(Error::Config(
                "the credentials bundle has no encryption private key".into(),
            ));
        }
        Self::new(
            &bundle.api_base_url,
            api_key,
            &bundle.encryption_key.private_key,
        )
    }

    // -- Public API -------------------------------------------------------------------------------

    /// Lists the user's accounts with all sensitive fields decrypted.
    pub fn get_accounts(&self) -> Result<Vec<Account>> {
        self.get_account_wires()?
            .iter()
            .map(|w| self.map_account(w))
            .collect()
    }

    /// Returns a page of an account's statement, newest first, with decrypted fields.
    pub fn get_transactions(
        &self,
        account_id: &str,
        query: &TransactionQuery,
    ) -> Result<TransactionPage> {
        let mut params: Vec<(&str, String)> = Vec::new();
        if let Some(from) = &query.from {
            params.push(("from", from.clone()));
        }
        if let Some(to) = &query.to {
            params.push(("to", to.clone()));
        }
        if let Some(limit) = query.limit {
            params.push(("limit", limit.to_string()));
        }
        if let Some(offset) = query.offset {
            params.push(("offset", offset.to_string()));
        }

        let path = format!("/api/accounts/{}/transactions", urlencode(account_id));
        let page: TransactionPageWire = self.get_json_query(&path, &params)?;
        let items = page
            .items
            .iter()
            .map(|t| self.map_transaction(t))
            .collect::<Result<Vec<_>>>()?;
        Ok(TransactionPage {
            items,
            total: page.total,
        })
    }

    /// Lists the user's bank connections.
    pub fn get_connections(&self) -> Result<Vec<Connection>> {
        let wires: Vec<ConnectionWire> = self.get_json("/api/connections")?;
        Ok(wires
            .into_iter()
            .map(|c| Connection {
                session_id: c.session_id,
                aspsp_name: c.aspsp_name,
                aspsp_country: c.aspsp_country,
                valid_until: c.valid_until,
                status: c.status,
                account_count: c.account_count,
                last_synced_at: c.last_synced_at,
                psu_type: c.psu_type,
            })
            .collect())
    }

    /// Triggers an online sync of one account.
    ///
    /// Decrypts that account's Enable Banking uid and posts it, so the service can fetch fresh data
    /// without ever holding the uid in plaintext.
    pub fn sync(&self, account_id: &str) -> Result<SyncResult> {
        let wires = self.get_account_wires()?;
        let account = wires
            .iter()
            .find(|a| a.id == account_id)
            .ok_or_else(|| Error::AccountNotFound(account_id.to_string()))?;
        let uid = self.decrypt_uid(account)?.ok_or(Error::NoActiveSession)?;

        let path = format!("/api/accounts/{}/sync", urlencode(account_id));
        let result: SyncResultWire = self.post_json(&path, &serde_json::json!({ "uid": uid }))?;
        Ok(SyncResult {
            new_transactions: result.new_transactions,
            total_fetched: result.total_fetched,
        })
    }

    /// Triggers an online sync of every account that has an active session.
    pub fn sync_all(&self) -> Result<SyncAllResult> {
        let wires = self.get_account_wires()?;
        let mut items = Vec::new();
        for a in &wires {
            if let Some(uid) = self.decrypt_uid(a)? {
                items.push(serde_json::json!({ "accountId": a.id, "uid": uid }));
            }
        }

        let result: SyncAllResultWire =
            self.post_json("/api/sync", &serde_json::json!({ "items": items }))?;
        Ok(SyncAllResult {
            accounts: result.accounts,
            new_transactions: result.new_transactions,
        })
    }

    // -- Internals --------------------------------------------------------------------------------

    fn get_account_wires(&self) -> Result<Vec<AccountWire>> {
        self.get_json("/api/accounts")
    }

    fn decrypt_uid(&self, account: &AccountWire) -> Result<Option<String>> {
        let dec: Option<UidEnc> =
            envelope::decrypt_to(&self.private_key, account.uid_enc.as_deref())?;
        Ok(dec.and_then(|d| d.uid))
    }

    fn map_account(&self, a: &AccountWire) -> Result<Account> {
        let acc: AccountEnc =
            envelope::decrypt_to(&self.private_key, a.enc.as_deref())?.unwrap_or_default();
        let name: DisplayNameEnc =
            envelope::decrypt_to(&self.private_key, a.display_name_enc.as_deref())?
                .unwrap_or_default();

        let mut balances = Vec::with_capacity(a.balances.len());
        for b in &a.balances {
            let dec: BalanceEnc =
                envelope::decrypt_to(&self.private_key, b.enc.as_deref())?.unwrap_or_default();
            balances.push(Balance {
                type_: b.r#type.clone(),
                name: dec.name,
                amount: dec.amount.unwrap_or_else(|| "0".into()),
                currency: b.currency.clone(),
                reference_date: b.reference_date.clone(),
            });
        }

        Ok(Account {
            id: a.id.clone(),
            aspsp_name: a.aspsp_name.clone(),
            aspsp_country: a.aspsp_country.clone(),
            currency: a.currency.clone(),
            account_type: a.account_type.clone(),
            bic: a.bic.clone(),
            needs_reconnect: a.needs_reconnect,
            iban: acc.iban,
            bban: acc.bban,
            owner_name: acc.owner_name,
            account_name: acc.account_name,
            product: acc.product,
            display_name: name.display_name,
            balances,
        })
    }

    fn map_transaction(&self, t: &TransactionWire) -> Result<Transaction> {
        let d: TransactionEnc =
            envelope::decrypt_to(&self.private_key, t.enc.as_deref())?.unwrap_or_default();
        Ok(Transaction {
            id: t.id.clone(),
            currency: t.currency.clone(),
            credit_debit_indicator: t.credit_debit_indicator.clone(),
            status: t.status.clone(),
            booking_date: t.booking_date.clone(),
            value_date: t.value_date.clone(),
            transaction_date: t.transaction_date.clone(),
            bank_transaction_code: t.bank_transaction_code.clone(),
            amount: d.amount.unwrap_or_else(|| "0".into()),
            creditor_name: d.creditor_name,
            creditor_iban: d.creditor_iban,
            creditor_bban: d.creditor_bban,
            creditor_agent_bic: d.creditor_agent_bic,
            debtor_name: d.debtor_name,
            debtor_iban: d.debtor_iban,
            debtor_bban: d.debtor_bban,
            debtor_agent_bic: d.debtor_agent_bic,
            remittance_information: d.remittance_information,
            note: d.note,
            reference_number: d.reference_number,
            exchange_rate: d.exchange_rate,
            merchant_category_code: d.merchant_category_code,
            balance_after_transaction: d.balance_after,
            balance_after_currency: d.balance_after_currency,
        })
    }

    fn get_json<T: DeserializeOwned>(&self, path: &str) -> Result<T> {
        self.get_json_query(path, &[])
    }

    fn get_json_query<T: DeserializeOwned>(
        &self,
        path: &str,
        params: &[(&str, String)],
    ) -> Result<T> {
        let mut req = self
            .agent
            .get(&format!("{}{}", self.base_url, path))
            .set("X-Api-Key", &self.api_key);
        for (k, v) in params {
            req = req.query(k, v);
        }
        let resp = req
            .call()
            .map_err(|e| Error::Http(format!("GET {path} failed: {e}")))?;
        resp.into_json()
            .map_err(|e| Error::Http(format!("GET {path} decode failed: {e}")))
    }

    fn post_json<T: DeserializeOwned, B: Serialize>(&self, path: &str, body: &B) -> Result<T> {
        let resp = self
            .agent
            .post(&format!("{}{}", self.base_url, path))
            .set("X-Api-Key", &self.api_key)
            .send_json(body)
            .map_err(|e| Error::Http(format!("POST {path} failed: {e}")))?;
        resp.into_json()
            .map_err(|e| Error::Http(format!("POST {path} decode failed: {e}")))
    }
}

/// Minimal percent-encoding for a single path segment (alnum and `-._~` pass through).
fn urlencode(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for b in s.bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'.' | b'_' | b'~' => {
                out.push(b as char)
            }
            _ => out.push_str(&format!("%{b:02X}")),
        }
    }
    out
}
