
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect } from 'chai';

import { resolveBundle } from '../lib/client.js';

// Vendored under test/fixtures so it survives the subtree-split to the mirror repo.
import { fileURLToPath } from 'node:url';
import { dirname } from 'node:path';
const FIXTURES = join(dirname(fileURLToPath(import.meta.url)), 'fixtures');
const BUNDLE = readFileSync(join(FIXTURES, 'credentials.json'), 'utf8');
// The fixture bundle ships with this apiBaseUrl.
const BUNDLE_URL = 'http://localhost:8081';

describe('resolveBundle()', () => {
  it('uses the bundle apiBaseUrl when no override is set', () => {
    expect(resolveBundle({ bundle: BUNDLE }).apiBaseUrl).to.equal(BUNDLE_URL);
  });

  it('applies a base-URL override, trimming trailing slashes', () => {
    const bundle = resolveBundle({
      bundle: BUNDLE,
      baseUrlOverride: 'https://api.staging.open-banking.io/',
    });
    expect(bundle.apiBaseUrl).to.equal('https://api.staging.open-banking.io');
    // The rest of the bundle is untouched.
    expect(bundle.apiKey).to.not.equal('');
    expect(bundle.privateKey).to.not.equal('');
  });

  it('ignores a blank/whitespace override and keeps the bundle URL', () => {
    expect(resolveBundle({ bundle: BUNDLE, baseUrlOverride: '   ' }).apiBaseUrl).to.equal(BUNDLE_URL);
    expect(resolveBundle({ bundle: BUNDLE, baseUrlOverride: '' }).apiBaseUrl).to.equal(BUNDLE_URL);
  });

  it('still rejects an invalid bundle', () => {
    expect(() => resolveBundle({ bundle: 'not json' })).to.throw(/not valid JSON/);
  });

  it('rejects an override that is not a URL', () => {
    expect(() => resolveBundle({ bundle: BUNDLE, baseUrlOverride: 'not a url' })).to.throw(
      /must be a valid URL/,
    );
  });

  it('rejects a non-http(s) override', () => {
    expect(() => resolveBundle({ bundle: BUNDLE, baseUrlOverride: 'ftp://x.test' })).to.throw(
      /must be an http\(s\) URL/,
    );
  });

  it('rejects a bundle whose apiBaseUrl is not a URL', () => {
    const bad = JSON.stringify({
      apiBaseUrl: 'nonsense',
      apiKey: 'k',
      encryptionKey: { privateKey: 'p' },
    });
    expect(() => resolveBundle({ bundle: bad })).to.throw(/must be a valid URL/);
  });
});
