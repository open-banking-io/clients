## What & why
<!-- Briefly describe the change and the motivation. -->

## Affected SDK(s)
<!-- The "Changed packages" bot will confirm. Check what you touched: -->
- [ ] dotnet · [ ] node · [ ] python · [ ] rust · [ ] go · [ ] java · [ ] ruby · [ ] php
- [ ] shared (`fixtures/`, workflows, docs)

## Checklist
- [ ] Tests pass locally for the affected SDK(s) (see `CONTRIBUTING.md`)
- [ ] Linters/formatters pass
- [ ] If the envelope/wire format changed, I regenerated `fixtures/` (`node tools/generate-fixtures.mjs`)
- [ ] I did **not** bump package versions (releases are cut via Actions → Release)

## Security checklist
- [ ] No secrets, credentials, or session uids in code, logs, or error messages
- [ ] Money uses decimal/exact types, never float
- [ ] No disabled TLS verification; no new network egress beyond the documented API
