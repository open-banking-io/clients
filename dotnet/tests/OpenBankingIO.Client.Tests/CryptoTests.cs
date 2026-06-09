using System.Security.Cryptography;

namespace OpenBankingIO.Client.Tests;

/// <summary>
/// Proves the .NET envelope decryptor interoperates with the service's wire format by decrypting the
/// shared, language-agnostic fixtures to the committed expected plaintext (same vectors all SDKs use).
/// </summary>
public class CryptoTests
{
    [Fact]
    public void Decrypts_Shared_Fixtures_To_Expected_Plaintext()
    {
        var kp = Fixtures.ReadJson("keypair.json");
        using var key = ECDiffieHellman.Create();
        key.ImportPkcs8PrivateKey(Convert.FromBase64String(kp.GetProperty("privateKeyPkcs8B64").GetString()!), out _);

        var env = Fixtures.ReadJson("envelopes.json");
        var exp = Fixtures.ReadJson("expected.json");

        var account = Envelope.DecryptTo<AccountEnc>(key, env.GetProperty("account").GetString())!;
        Assert.Equal(exp.GetProperty("account").GetProperty("iban").GetString(), account.Iban);
        Assert.Equal(exp.GetProperty("account").GetProperty("ownerName").GetString(), account.OwnerName);
        Assert.Equal(exp.GetProperty("account").GetProperty("bban").GetString(), account.Bban);

        var transaction = Envelope.DecryptTo<TransactionEnc>(key, env.GetProperty("transaction").GetString())!;
        Assert.Equal(exp.GetProperty("transaction").GetProperty("amount").GetString(), transaction.Amount);
        Assert.Equal(exp.GetProperty("transaction").GetProperty("creditorName").GetString(), transaction.CreditorName);
        Assert.Equal(exp.GetProperty("transaction").GetProperty("merchantCategoryCode").GetString(), transaction.MerchantCategoryCode);

        var uid = Envelope.DecryptTo<UidEnc>(key, env.GetProperty("uid").GetString())!;
        Assert.Equal(exp.GetProperty("uid").GetProperty("uid").GetString(), uid.Uid);

        var balance = Envelope.DecryptTo<BalanceEnc>(key, env.GetProperty("balance").GetString())!;
        Assert.Equal(exp.GetProperty("balances").GetProperty("ITBD").GetProperty("amount").GetString(), balance.Amount);
    }

    [Fact]
    public void Wrong_Key_Fails_Authentication_Tag()
    {
        using var other = ECDiffieHellman.Create(ECCurve.NamedCurves.nistP256);
        var env = Fixtures.ReadJson("envelopes.json");
        var bytes = Convert.FromBase64String(env.GetProperty("account").GetString()!);
        Assert.Throws<AuthenticationTagMismatchException>(() => Envelope.Decrypt(other, bytes));
    }
}
