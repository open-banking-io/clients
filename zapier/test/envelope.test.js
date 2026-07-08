'use strict';

const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const { expect } = require('chai');

const { decryptTo, importPrivateKey } = require('../lib/envelope');

// Decrypt the shared cross-language test vectors and assert byte-identical results
// to the other SDKs. This proves the port matches the canonical implementation.
// The vectors are vendored under test/fixtures so they survive the subtree-split
// to the open-banking-io/zapier mirror repo (the monorepo's fixtures/ does not).

const FIXTURES = join(__dirname, 'fixtures');

function loadJson(name) {
  return JSON.parse(readFileSync(join(FIXTURES, name), 'utf8'));
}

const keypair = loadJson('keypair.json');
const envelopes = loadJson('envelopes.json');
const expected = loadJson('expected.json');

describe('envelope decryptor (ported to global Web Crypto)', () => {
  it('decrypts the account envelope', async () => {
    const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
    expect(await decryptTo(key, envelopes.account)).to.deep.equal(expected.account);
  });

  it('decrypts the display name envelope', async () => {
    const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
    expect(await decryptTo(key, envelopes.displayName)).to.deep.equal(expected.displayName);
  });

  it('decrypts the session uid envelope', async () => {
    const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
    expect(await decryptTo(key, envelopes.uid)).to.deep.equal(expected.uid);
  });

  it('decrypts the transaction envelope', async () => {
    const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
    expect(await decryptTo(key, envelopes.transaction)).to.deep.equal(expected.transaction);
  });

  it('decrypts the balance envelope to one of the expected balances', async () => {
    const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
    const decrypted = await decryptTo(key, envelopes.balance);
    const balances = Object.values(expected.balances);
    expect(balances).to.deep.include(decrypted);
  });

  it('rejects a tampered envelope', async () => {
    const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
    let threw = false;
    try {
      await decryptTo(key, 'AAAA');
    } catch {
      threw = true;
    }
    expect(threw).to.be.true;
  });

  it('returns null for a missing envelope', async () => {
    const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
    expect(await decryptTo(key, null)).to.be.null;
    expect(await decryptTo(key, undefined)).to.be.null;
  });
});
