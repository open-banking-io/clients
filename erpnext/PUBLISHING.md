# Publishing to the Frappe Cloud Marketplace

Maintainer runbook for listing `erpnext_open_banking` on the Frappe Cloud
Marketplace. The code side is done and released; the remaining steps are a
Frappe Cloud **web-UI flow under the account that owns the org** — Frappe Cloud
has no API/token path for marketplace submission, so this can't be scripted.
Everything below is pre-filled to copy-paste.

App: **ERPNext Open Banking** · id `erpnext_open_banking`
Repo to connect: **https://github.com/open-banking-io/erpnext** (branch `main`, latest release `v0.1.1`)

---

## Ordered steps (developer.frappecloud.com)

1. **Become a Publisher** — Frappe Cloud dashboard → Settings → Profile → *Become a Publisher*.
2. **Add App** — Marketplace tab → **+ Add App** → **Add from GitHub** → authorize →
   pick `open-banking-io/erpnext`.
3. **Frappe compatibility** — select **Version 15** (the app is tested on ERPNext v15; requires-python ≥3.10).
4. **Fill the Overview / listing fields** (draft status) — use the content block below.
5. **Publish a release** — select **v0.1.1**. The automated **App Auditor** runs
   (Pass / Warn / Needs-Improvement / Fail); a Fail blocks, a Warn flags for review.
6. **Await approval** — Frappe reviews; on approval it goes live at cloud.frappe.io/marketplace.

---

## Paste-ready listing content

- **App title:** `ERPNext Open Banking`
- **Short description** (40–80 chars, required):
  `Sync bank transactions into ERPNext via open-banking.io (PSD2).`   (63 chars)
- **Category:** Accounting / Banking (choose "Accounting" if Banking isn't offered)
- **Publisher:** open-banking.io
- **Logo** (≥200×200, square, no text): **https://open-banking.io/icon-512.png**
  (512×512 PNG — same asset used for the Zapier listing)
- **Support URL:** `https://github.com/open-banking-io/clients/issues`
  (checked: open-banking.io has no dedicated /support page — it SPA-falls-back to the home shell.
  Use the issues tracker, or `mailto:info@open-banking.io` if the form prefers an email.)
- **Privacy Policy URL:** `https://open-banking.io/privacy`  ✅ verified (real page, title "Privacy Policy | Open Banking Access")
- **Long description** (from the README — the marketplace renders markdown):
  > Sync your bank transactions directly into ERPNext's **Bank Transaction** doctype,
  > where they appear in the **Bank Reconciliation Tool**, ready to match against payment
  > entries and invoices.
  >
  > - **No eIDAS/QWAC certificates** — open-banking.io handles PSD2 compliance
  > - **Zero-knowledge encryption** — data is decrypted locally inside your ERPNext; the service only stores ciphertext it can't read
  > - **Automated sync** — the scheduler pulls new transactions every 6 hours, or on-demand from the Bank Account form
  > - **Self-hosted friendly** — works on Frappe Cloud or your own bench
- **Demo video** (required by review): record a ≤2-min screen capture — install the app,
  paste a credentials bundle in *Open Banking Settings* → Test Connection, create an
  *Open Banking Connection*, click **Sync from Open Banking**, show the transactions land
  in the Bank Reconciliation Tool. (This is the exact flow the smoke test proved works.)

---

## Compliance status (what the App Auditor checks)

| Requirement | Status |
|---|---|
| Installs cleanly on current ERPNext v15 | ✅ verified end-to-end on a real bench |
| Passing CI with linters + server tests | ✅ mirror CI (ruff + pytest, 3.10/3.11/3.13) — v0.1.1 |
| MIT license present | ✅ `LICENSE` |
| Settings doctype for global config | ✅ Open Banking Settings |
| Doesn't override base Frappe (auth/etc.) | ✅ hooks only (scheduler + doctype JS) |
| App metadata (title/publisher/description/license/email) | ✅ `hooks.py` |
| Unique app name | ✅ `erpnext_open_banking` |

---

## The two things only a human can do
1. The UI submission above (the org owner's Frappe Cloud login).
2. The demo video.

Everything else — installable versioned mirror, green CI, compliant metadata, listing
copy, logo — is ready.
