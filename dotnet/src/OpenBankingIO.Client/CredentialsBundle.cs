namespace OpenBankingIO.Client;

/// <summary>The credentials bundle exported from open-banking.io (API key + encryption private key).</summary>
public sealed record CredentialsBundle
{
    public string Service { get; init; } = "";
    public string ApiBaseUrl { get; init; } = "";
    public string User { get; init; } = "";
    public string? ApiKey { get; init; }
    public EncryptionKey EncryptionKey { get; init; } = new();
}

public sealed record EncryptionKey
{
    public string Scheme { get; init; } = "";
    public string Curve { get; init; } = "";
    public string PrivateKeyFormat { get; init; } = "";
    /// <summary>The PKCS#8 private key, base64-encoded.</summary>
    public string PrivateKey { get; init; } = "";
    public string? PublicKey { get; init; }
}
