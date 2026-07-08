# Changelog

## 1.0.1

- Explicit input handling app-wide (`cleanInputData: false`): empty values fall back to sane defaults.
- Base URLs (credentials bundle and override) are validated as http(s) URLs with clear error messages.
- Auth fields link to the setup guide.

## 1.0.0

Initial release.

- **New Transaction** polling trigger with incremental cursor and pending-transaction handling.
- **Find Account** and **Find Transactions** searches with decrypted account and statement data.
- **Sync Account** action to pull fresh transactions from the bank.
- Zero-knowledge decryption: all sensitive data is decrypted inside Zapier with your private key; the service only ever stores ciphertext.
