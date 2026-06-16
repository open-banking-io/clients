# Security Policy

`open-banking-io` is a family of **zero-knowledge** banking client SDKs: they hold your private key and
decrypt account data locally, so the service never sees plaintext. We take vulnerabilities seriously.

## Reporting a vulnerability

**Please do not open a public issue, PR, or discussion for security vulnerabilities.**

Report privately via either:

- **GitHub Private Vulnerability Reporting** — the *Report a vulnerability* button under this repository's
  **Security** tab (preferred), or
- **Email** — `security@open-banking.io`

Please include: a description, affected SDK(s) and version(s), reproduction steps or a proof of concept,
and an impact assessment.

Our response targets:

- **Acknowledgement** within **48 hours**.
- **Initial assessment** (severity and whether it's in scope) within **7 days**.
- **Fix and coordinated disclosure** as quickly as the severity warrants — we aim for within **90 days**,
  and faster for issues under active exploitation.

We keep you updated throughout and are happy to credit you in the release notes.

## Scope

In scope: all SDK code (.NET, Node, Python, Rust, Go, Java, Ruby, PHP) — the envelope decryption, key
handling, and HTTP client configuration (TLS, timeouts).

Out of scope (report to the relevant party): the open-banking.io backend service itself, and
vulnerabilities in third-party dependencies (report upstream; we'll bump once a fix is released).

## Cryptography

The envelope scheme is **ephemeral ECDH on NIST P-256 → HKDF-SHA256 (salt = 32 zero bytes, info
`bank.core.ci/zk/v1`) → AES-256-GCM**. All eight SDKs are verified against the same shared test vectors
(`fixtures/`) to guarantee identical, interoperable behaviour. See [`THREAT_MODEL.md`](THREAT_MODEL.md)
for trust boundaries and assumptions.

## Using the SDKs safely

- **Never commit your credentials bundle** — it contains your private key. Store it in a secrets manager.
- Keep the SDK and its dependencies up to date (Dependabot is enabled here).
- Use `https://` endpoints only (the SDKs verify TLS certificates by default).

## Supported versions

We provide security fixes for the **latest released minor** of each SDK. Please upgrade before reporting
issues against older versions.
