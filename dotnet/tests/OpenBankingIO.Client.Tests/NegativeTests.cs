using System.Security.Cryptography;

namespace OpenBankingIO.Client.Tests;

/// <summary>
/// Failure-path coverage: malformed/truncated envelopes, bad keys, and a non-2xx HTTP response.
/// These lock the library's "fail loud, never silently mis-decrypt" guarantees.
/// </summary>
public class NegativeTests
{
    private static ECDiffieHellman FixtureKey()
    {
        var kp = Fixtures.ReadJson("keypair.json");
        var key = ECDiffieHellman.Create();
        key.ImportPkcs8PrivateKey(Convert.FromBase64String(kp.GetProperty("privateKeyPkcs8B64").GetString()!), out _);
        return key;
    }

    [Fact]
    public void Truncated_Envelope_Throws_Format()
    {
        using var key = FixtureKey();
        // Shorter than version(1) + ephemeralPublicKey(65) + nonce(12) + tag(16) = 94 bytes.
        var tooShort = new byte[10];
        tooShort[0] = 0x01;

        Assert.Throws<FormatException>(() => Envelope.Decrypt(key, tooShort));
    }

    [Fact]
    public void Empty_Envelope_Throws_Format()
    {
        using var key = FixtureKey();
        Assert.Throws<FormatException>(() => Envelope.Decrypt(key, []));
    }

    [Fact]
    public void Wrong_Version_Byte_Throws_Format()
    {
        using var key = FixtureKey();
        // A full-length, otherwise well-formed buffer but with an unsupported version marker (0x02).
        var env = new byte[1 + 65 + 12 + 16 + 4];
        env[0] = 0x02;

        Assert.Throws<FormatException>(() => Envelope.Decrypt(key, env));
    }

    [Fact]
    public void Tampered_Ciphertext_Throws_AuthenticationTagMismatch()
    {
        using var key = FixtureKey();
        var env = Fixtures.ReadJson("envelopes.json");
        var bytes = Convert.FromBase64String(env.GetProperty("account").GetString()!);
        // Flip a bit in the ciphertext (past version+pubkey+nonce+tag) — GCM must reject it.
        bytes[^1] ^= 0xFF;

        Assert.Throws<AuthenticationTagMismatchException>(() => Envelope.Decrypt(key, bytes));
    }

    [Fact]
    public void Invalid_Pkcs8_Key_String_Throws()
    {
        // A syntactically valid base64 string that is not a valid PKCS#8 private key.
        var notAKey = Convert.ToBase64String("this is not a pkcs8 key"u8.ToArray());

        Assert.ThrowsAny<CryptographicException>(() =>
            new OpenBankingClient("https://example.test", "api-key", notAKey));
    }

    [Fact]
    public void Non_Base64_Key_String_Throws_Format()
    {
        Assert.Throws<FormatException>(() =>
            new OpenBankingClient("https://example.test", "api-key", "!!! not base64 !!!"));
    }

    [Fact]
    public async Task Non_2xx_Response_Throws_HttpRequestException()
    {
        var creds = Fixtures.ReadJson("credentials.json");
        var apiKey = creds.GetProperty("apiKey").GetString()!;
        var privateKey = creds.GetProperty("encryptionKey").GetProperty("privateKey").GetString()!;

        using var server = new MockServer(apiKey);
        using var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey);

        // The "__error__" sentinel account makes the mock return HTTP 500;
        // GetFromJsonAsync surfaces that as a non-success HttpRequestException.
        await Assert.ThrowsAsync<HttpRequestException>(
            () => client.GetTransactionsAsync("__error__"));
    }
}
