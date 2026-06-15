import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { describe, expect, it } from 'vitest';

import { decryptTo, importPrivateKey } from '../nodes/OpenBankingIo/shared/envelope';

// Decrypt the shared cross-language test vectors and assert byte-identical results to the
// other SDKs. This proves the global-Web-Crypto port matches the canonical implementation.

const FIXTURES = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'fixtures');

function loadJson<T>(name: string): T {
	return JSON.parse(readFileSync(join(FIXTURES, name), 'utf8')) as T;
}

const keypair = loadJson<{ privateKeyPkcs8B64: string }>('keypair.json');
const envelopes = loadJson<Record<string, string>>('envelopes.json');
const expected = loadJson<Record<string, unknown>>('expected.json');

describe('envelope decryptor (ported to global Web Crypto)', () => {
	it('decrypts the account envelope', async () => {
		const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
		expect(await decryptTo(key, envelopes.account)).toEqual(expected.account);
	});

	it('decrypts the display name envelope', async () => {
		const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
		expect(await decryptTo(key, envelopes.displayName)).toEqual(expected.displayName);
	});

	it('decrypts the session uid envelope', async () => {
		const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
		expect(await decryptTo(key, envelopes.uid)).toEqual(expected.uid);
	});

	it('decrypts the transaction envelope', async () => {
		const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
		expect(await decryptTo(key, envelopes.transaction)).toEqual(expected.transaction);
	});

	it('decrypts the balance envelope to one of the expected balances', async () => {
		const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
		const decrypted = await decryptTo(key, envelopes.balance);
		const balances = Object.values(expected.balances as Record<string, unknown>);
		expect(balances).toContainEqual(decrypted);
	});

	it('rejects a tampered envelope', async () => {
		const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
		await expect(decryptTo(key, 'AAAA')).rejects.toThrow();
	});

	it('returns null for a missing envelope', async () => {
		const key = await importPrivateKey(keypair.privateKeyPkcs8B64);
		expect(await decryptTo(key, null)).toBeNull();
		expect(await decryptTo(key, undefined)).toBeNull();
	});
});
