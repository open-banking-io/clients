//! Integration tests against a mock API served from the shared fixtures.

use std::fs;
use std::sync::mpsc;
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Duration;

use open_banking_io::{OpenBankingClient, TransactionQuery};
use serde_json::Value;
use tiny_http::{Header, Method, Response, Server};

fn fixtures() -> String {
    concat!(env!("CARGO_MANIFEST_DIR"), "/../fixtures/").to_string()
}

fn read_fixture(name: &str) -> String {
    fs::read_to_string(format!("{}{}", fixtures(), name)).unwrap()
}

fn read_json(name: &str) -> Value {
    serde_json::from_str(&read_fixture(name)).unwrap()
}

struct MockApi {
    base_url: String,
    api_key: String,
    private_key: String,
    last_sync_body: Arc<Mutex<Option<Value>>>,
    last_user_agent: Arc<Mutex<Option<String>>>,
}

fn start_mock() -> MockApi {
    let api_key = read_json("credentials.json")["apiKey"]
        .as_str()
        .unwrap()
        .to_string();
    let private_key = read_json("keypair.json")["privateKeyPkcs8B64"]
        .as_str()
        .unwrap()
        .to_string();

    let last_sync_body = Arc::new(Mutex::new(None));
    let last_user_agent = Arc::new(Mutex::new(None));
    let server = Server::http("127.0.0.1:0").unwrap();
    let port = server.server_addr().to_ip().unwrap().port();
    let base_url = format!("http://127.0.0.1:{port}");

    let expected_key = api_key.clone();
    let sync_body = Arc::clone(&last_sync_body);
    let user_agent = Arc::clone(&last_user_agent);
    let (ready_tx, ready_rx) = mpsc::channel();
    thread::spawn(move || {
        ready_tx.send(()).ok();
        for mut request in server.incoming_requests() {
            if let Some(ua) = request
                .headers()
                .iter()
                .find(|h| h.field.as_str().as_str().eq_ignore_ascii_case("User-Agent"))
            {
                *user_agent.lock().unwrap() = Some(ua.value.as_str().to_string());
            }

            let authorized = request.headers().iter().any(|h| {
                h.field.as_str().as_str().eq_ignore_ascii_case("X-Api-Key")
                    && h.value.as_str() == expected_key
            });
            if !authorized {
                let _ = request.respond(json_response(401, r#"{"error":"unauthorized"}"#));
                continue;
            }

            let method = request.method().clone();
            let path = request.url().split('?').next().unwrap_or("").to_string();

            let response = if method == Method::Get && path == "/api/accounts" {
                json_response(200, &read_fixture("api/accounts.json"))
            } else if method == Method::Get
                && path.starts_with("/api/accounts/")
                && path.ends_with("/transactions")
            {
                json_response(200, &read_fixture("api/transactions.json"))
            } else if method == Method::Get && path == "/api/connections" {
                json_response(200, &read_fixture("api/connections.json"))
            } else if method == Method::Post
                && path.starts_with("/api/accounts/")
                && path.ends_with("/sync")
            {
                let mut body = String::new();
                request.as_reader().read_to_string(&mut body).ok();
                *sync_body.lock().unwrap() = serde_json::from_str(&body).ok();
                json_response(200, &read_fixture("api/sync.json"))
            } else if method == Method::Post && path == "/api/sync" {
                let mut body = String::new();
                request.as_reader().read_to_string(&mut body).ok();
                *sync_body.lock().unwrap() = serde_json::from_str(&body).ok();
                json_response(200, &read_fixture("api/sync-all.json"))
            } else {
                json_response(404, r#"{"error":"not found"}"#)
            };
            let _ = request.respond(response);
        }
    });
    ready_rx.recv().unwrap();

    MockApi {
        base_url,
        api_key,
        private_key,
        last_sync_body,
        last_user_agent,
    }
}

fn json_response(status: u32, body: &str) -> Response<std::io::Cursor<Vec<u8>>> {
    Response::from_string(body)
        .with_status_code(status)
        .with_header(Header::from_bytes(&b"Content-Type"[..], &b"application/json"[..]).unwrap())
}

impl MockApi {
    fn client(&self) -> OpenBankingClient {
        OpenBankingClient::new(&self.base_url, &self.api_key, &self.private_key).unwrap()
    }
}

#[test]
fn get_accounts_decrypts_iban_owner_display_name_and_three_balances() {
    let api = start_mock();
    let accounts = api.client().get_accounts().unwrap();
    assert_eq!(accounts.len(), 1);
    let a = &accounts[0];
    assert_eq!(a.iban.as_deref(), Some("DK6466952001724927"));
    assert_eq!(a.owner_name.as_deref(), Some("Tatic ApS"));
    assert_eq!(a.display_name.as_deref(), Some("Drift"));
    assert_eq!(a.balances.len(), 3);
    let itbd = a.balances.iter().find(|b| b.type_ == "ITBD").unwrap();
    let itav = a.balances.iter().find(|b| b.type_ == "ITAV").unwrap();
    assert_eq!(itbd.amount, "828.13");
    assert_eq!(itav.amount, "633.90");
}

#[test]
fn get_transactions_decrypts_amount_and_creditor() {
    let api = start_mock();
    let page = api
        .client()
        .get_transactions(
            "11111111-1111-4111-8111-111111111111",
            &TransactionQuery {
                limit: Some(50),
                ..Default::default()
            },
        )
        .unwrap();
    assert_eq!(page.total, 1);
    let t = &page.items[0];
    assert_eq!(t.amount, "194.23");
    assert_eq!(t.creditor_name.as_deref(), Some("One.com"));
}

#[test]
fn get_connections_works() {
    let api = start_mock();
    let conns = api.client().get_connections().unwrap();
    assert_eq!(conns.len(), 1);
    assert_eq!(conns[0].aspsp_name, "Lunar");
    assert_eq!(conns[0].status, "Active");
}

#[test]
fn sync_posts_a_body_containing_the_decrypted_uid() {
    let api = start_mock();
    let result = api
        .client()
        .sync("11111111-1111-4111-8111-111111111111")
        .unwrap();
    assert_eq!(result.total_fetched, 1);
    let body = api.last_sync_body.lock().unwrap().clone().unwrap();
    assert_eq!(
        body,
        serde_json::json!({ "uid": "c5d93aa7-5e23-4da0-ba88-42b9a584492c" })
    );
}

#[test]
fn sync_all_posts_items_with_the_decrypted_uid() {
    let api = start_mock();
    let result = api.client().sync_all().unwrap();
    assert_eq!(result.accounts, 1);
    let body = api.last_sync_body.lock().unwrap().clone().unwrap();
    assert_eq!(
        body,
        serde_json::json!({
            "items": [{
                "accountId": "11111111-1111-4111-8111-111111111111",
                "uid": "c5d93aa7-5e23-4da0-ba88-42b9a584492c"
            }]
        })
    );
}

#[test]
fn requests_send_the_open_banking_io_user_agent() {
    let api = start_mock();
    api.client().get_accounts().unwrap();
    let ua = api.last_user_agent.lock().unwrap().clone().unwrap();
    assert!(
        ua.starts_with("open-banking-io/rust/"),
        "unexpected User-Agent: {ua}"
    );
}

#[test]
fn builder_with_custom_timeouts_builds_a_working_client() {
    let api = start_mock();
    let client = OpenBankingClient::builder(&api.base_url, &api.api_key, &api.private_key)
        .connect_timeout(Duration::from_secs(5))
        .timeout(Duration::from_secs(15))
        .build()
        .unwrap();
    let accounts = client.get_accounts().unwrap();
    assert_eq!(accounts.len(), 1);
    // Custom-timeout agent still emits the SDK User-Agent.
    let ua = api.last_user_agent.lock().unwrap().clone().unwrap();
    assert!(
        ua.starts_with("open-banking-io/rust/"),
        "unexpected User-Agent: {ua}"
    );
}

#[test]
fn builder_with_a_custom_agent_builds_a_working_client() {
    let api = start_mock();
    let agent: ureq::Agent = ureq::Agent::config_builder()
        .timeout_connect(Some(Duration::from_secs(3)))
        .user_agent("open-banking-io/rust/custom-agent-test")
        .build()
        .into();
    let client = OpenBankingClient::builder(&api.base_url, &api.api_key, &api.private_key)
        .agent(agent)
        .build()
        .unwrap();
    let accounts = client.get_accounts().unwrap();
    assert_eq!(accounts.len(), 1);
    // The caller-supplied agent's User-Agent is used verbatim (not overridden by the SDK).
    let ua = api.last_user_agent.lock().unwrap().clone().unwrap();
    assert_eq!(ua, "open-banking-io/rust/custom-agent-test");
}

#[test]
fn builder_validates_like_new() {
    let api = start_mock();
    assert!(
        OpenBankingClient::builder("", &api.api_key, &api.private_key)
            .build()
            .is_err()
    );
    assert!(
        OpenBankingClient::builder(&api.base_url, "", &api.private_key)
            .build()
            .is_err()
    );
    assert!(OpenBankingClient::builder(&api.base_url, &api.api_key, "")
        .build()
        .is_err());
}

#[test]
fn a_wrong_api_key_errors() {
    let api = start_mock();
    let client = OpenBankingClient::new(&api.base_url, "wrong-key", &api.private_key).unwrap();
    assert!(client.get_accounts().is_err());
}

#[test]
fn a_wrong_private_key_errors_on_decryption() {
    let api = start_mock();
    let bad_key = "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgdB9vjCwdrF1FckSNDHrw8M7PNNkpUB/tAK/EgAbZWvihRANCAASgg8XOKlU9VeYCee9+tQKtDSkFze10CRTA4b2gGKDlHIFw+QTf1AkjAjLfWqCJ4BctUqQtAYs0v0Y90Bw1wsBF";
    let client = OpenBankingClient::new(&api.base_url, &api.api_key, bad_key).unwrap();
    assert!(client.get_accounts().is_err());
}
