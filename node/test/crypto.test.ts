import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { decryptTo, importPrivateKey } from "../src/index.js";

const fixtures = fileURLToPath(new URL("../../fixtures/", import.meta.url));
const read = <T>(name: string): T => JSON.parse(readFileSync(fixtures + name, "utf8")) as T;

const keypair = read<{ privateKeyPkcs8B64: string }>("keypair.json");
const envelopes = read<{
  account: string;
  displayName: string;
  uid: string;
  balance: string;
  transaction: string;
}>("envelopes.json");
const expected = read<{
  account: { iban: string; ownerName: string; bban: string };
  displayName: { displayName: string };
  uid: { uid: string };
  balances: { ITBD: { amount: string; name: string } };
  transaction: { amount: string; creditorName: string };
}>("expected.json");

const privKey = importPrivateKey(keypair.privateKeyPkcs8B64);

describe("envelope decryption (wire-format interop)", () => {
  it("decrypts the account envelope", async () => {
    const acc = await decryptTo<{ iban: string; ownerName: string; bban: string }>(
      await privKey,
      envelopes.account,
    );
    expect(acc?.iban).toBe(expected.account.iban);
    expect(acc?.ownerName).toBe(expected.account.ownerName);
    expect(acc?.bban).toBe(expected.account.bban);
  });

  it("decrypts the displayName envelope", async () => {
    const dn = await decryptTo<{ displayName: string }>(await privKey, envelopes.displayName);
    expect(dn?.displayName).toBe(expected.displayName.displayName);
  });

  it("decrypts the uid envelope", async () => {
    const uid = await decryptTo<{ uid: string }>(await privKey, envelopes.uid);
    expect(uid?.uid).toBe(expected.uid.uid);
  });

  it("decrypts the balance envelope (ITBD amount kept as string)", async () => {
    const bal = await decryptTo<{ amount: string; name: string }>(await privKey, envelopes.balance);
    expect(bal?.amount).toBe(expected.balances.ITBD.amount);
    expect(typeof bal?.amount).toBe("string");
    expect(bal?.name).toBe(expected.balances.ITBD.name);
  });

  it("decrypts the transaction envelope", async () => {
    const tx = await decryptTo<{ amount: string; creditorName: string }>(
      await privKey,
      envelopes.transaction,
    );
    expect(tx?.amount).toBe(expected.transaction.amount);
    expect(typeof tx?.amount).toBe("string");
    expect(tx?.creditorName).toBe(expected.transaction.creditorName);
  });
});
