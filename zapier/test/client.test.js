'use strict';

const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const { expect } = require('chai');

const { resolveBundle } = require('../lib/client');

// Vendored under test/fixtures so it survives the subtree-split to the mirror repo.
const FIXTURES = join(__dirname, 'fixtures');
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
});
