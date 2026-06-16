import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { describe, expect, it } from 'vitest';

import { resolveBundle } from '../nodes/OpenBankingIo/shared/client';

// `resolveBundle` parses the pasted bundle and applies the optional `baseUrlOverride` credential
// field, so a credential can point at staging/local without re-exporting a different bundle.

const FIXTURES = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'fixtures');
const BUNDLE = readFileSync(join(FIXTURES, 'credentials.json'), 'utf8');
// The fixture bundle ships with this apiBaseUrl.
const BUNDLE_URL = 'http://localhost:8081';

describe('resolveBundle()', () => {
	it('uses the bundle apiBaseUrl when no override is set', () => {
		expect(resolveBundle({ bundle: BUNDLE }).apiBaseUrl).toBe(BUNDLE_URL);
	});

	it('applies a base-URL override, trimming trailing slashes', () => {
		const bundle = resolveBundle({
			bundle: BUNDLE,
			baseUrlOverride: 'https://api.staging.open-banking.io/',
		});
		expect(bundle.apiBaseUrl).toBe('https://api.staging.open-banking.io');
		// The rest of the bundle is untouched.
		expect(bundle.apiKey).not.toBe('');
		expect(bundle.privateKey).not.toBe('');
	});

	it('ignores a blank/whitespace override and keeps the bundle URL', () => {
		expect(resolveBundle({ bundle: BUNDLE, baseUrlOverride: '   ' }).apiBaseUrl).toBe(BUNDLE_URL);
		expect(resolveBundle({ bundle: BUNDLE, baseUrlOverride: '' }).apiBaseUrl).toBe(BUNDLE_URL);
	});

	it('still rejects an invalid bundle', () => {
		expect(() => resolveBundle({ bundle: 'not json' })).toThrow(/not valid JSON/);
	});
});
