using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace OpenBankingIO.Client;

/// <summary>
/// Decrypts open-banking.io's zero-knowledge data envelopes.
/// Scheme: ephemeral ECDH on NIST P-256 → HKDF-SHA256 → AES-256-GCM.
/// Wire: <c>version(1)=0x01 | ephemeralPublicKeyRaw(65) | nonce(12) | tag(16) | ciphertext</c>.
/// Only the user's private key can decrypt — the service stores ciphertext it cannot read.
/// </summary>
internal static class Envelope
{
    private const int PointLen = 65;
    private const int CoordLen = 32;
    private const int NonceLen = 12;
    private const int TagLen = 16;
    private static readonly byte[] Info = Encoding.UTF8.GetBytes("bank.core.ci/zk/v1");

    public static byte[] Decrypt(ECDiffieHellman privateKey, ReadOnlySpan<byte> envelope)
    {
        if (envelope.Length < 1 + PointLen + NonceLen + TagLen || envelope[0] != 0x01)
            throw new FormatException("Invalid or unsupported envelope");

        var o = 1;
        var ephPub = envelope.Slice(o, PointLen); o += PointLen;
        var nonce = envelope.Slice(o, NonceLen); o += NonceLen;
        var tag = envelope.Slice(o, TagLen); o += TagLen;
        var ciphertext = envelope[o..];

        using var ephemeral = ECDiffieHellman.Create();
        try
        {
            ephemeral.ImportParameters(new ECParameters
            {
                Curve = ECCurve.NamedCurves.nistP256,
                Q = new ECPoint
                {
                    X = ephPub.Slice(1, CoordLen).ToArray(),
                    Y = ephPub.Slice(1 + CoordLen, CoordLen).ToArray(),
                },
            });
        }
        catch (CryptographicException e)
        {
            throw new FormatException("Invalid ephemeral public key", e);
        }

        var shared = privateKey.DeriveRawSecretAgreement(ephemeral.PublicKey);
        // salt: null == 32 zero bytes (RFC 5869), matching the browser's `new Uint8Array(32)`.
        var key = HKDF.DeriveKey(HashAlgorithmName.SHA256, shared, 32, salt: null, info: Info);

        var plaintext = new byte[ciphertext.Length];
        using (var aes = new AesGcm(key, TagLen))
        {
            aes.Decrypt(nonce, ciphertext, tag, plaintext);
        }

        CryptographicOperations.ZeroMemory(shared);
        CryptographicOperations.ZeroMemory(key);
        return plaintext;
    }

    /// <summary>Decrypts a base64 envelope and deserializes its JSON payload. Null in → default.</summary>
    public static T? DecryptTo<T>(ECDiffieHellman privateKey, string? envelopeBase64)
        => envelopeBase64 is null
            ? default
            : JsonSerializer.Deserialize<T>(Decrypt(privateKey, Convert.FromBase64String(envelopeBase64)), Json.Options);
}

internal static class Json
{
    public static readonly JsonSerializerOptions Options = new(JsonSerializerDefaults.Web);
}
