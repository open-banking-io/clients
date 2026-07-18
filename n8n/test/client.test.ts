import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { describe, expect, it, vi } from 'vitest';

import type { IHttpRequestOptions } from 'n8n-workflow';

import { apiRequest, resolveBundle, type RequestContext } from '../nodes/OpenBankingIo/shared/client';
import { USER_AGENT, VERSION } from '../nodes/OpenBankingIo/shared/version';

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

// `apiRequest` issues the authenticated HTTP call. It must tag every request with a versioned
// `User-Agent` so open-banking.io can identify traffic coming from this n8n community node — the
// same `open-banking-io/<sdk>/<version>` convention the other SDKs use.
describe('apiRequest()', () => {
	function mockContext() {
		const httpRequestWithAuthentication = vi.fn().mockResolvedValue({ ok: true });
		const ctx = { helpers: { httpRequestWithAuthentication } } as unknown as RequestContext;
		return { ctx, httpRequestWithAuthentication };
	}

	const bundle = { apiBaseUrl: 'https://api.example.test', apiKey: 'k', privateKey: 'p' };

	it('sends a User-Agent of open-banking-io/n8n/<version>', async () => {
		const { ctx, httpRequestWithAuthentication } = mockContext();

		await apiRequest(ctx, bundle, 'GET', '/api/accounts');

		expect(httpRequestWithAuthentication).toHaveBeenCalledTimes(1);
		// Invoked as `httpRequestWithAuthentication.call(ctx, CREDENTIALS_NAME, options)`, so the
		// recorded args are [credentialsName, options] (the `.call` thisArg is not an arg).
		const options = httpRequestWithAuthentication.mock.calls[0][1] as IHttpRequestOptions;
		// Imported from the same source as the client, so it can never drift from package.json.
		expect(USER_AGENT).toBe(`open-banking-io/n8n/${VERSION}`);
		expect(options.headers?.['User-Agent']).toBe(USER_AGENT);
	});
});
