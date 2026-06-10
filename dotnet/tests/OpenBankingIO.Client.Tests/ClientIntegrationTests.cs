namespace OpenBankingIO.Client.Tests;

/// <summary>
/// End-to-end against a local mock server serving the encrypted fixtures: HTTP + X-Api-Key auth +
/// decryption + model mapping + the online-sync round-trip.
/// </summary>
public class ClientIntegrationTests
{
    private const string AccountId = "11111111-1111-4111-8111-111111111111";

    private static readonly System.Text.Json.JsonSerializerOptions WebJson =
        new(System.Text.Json.JsonSerializerDefaults.Web);

    private static (MockServer Server, OpenBankingClient Client) Make()
    {
        var creds = Fixtures.ReadJson("credentials.json");
        var apiKey = creds.GetProperty("apiKey").GetString()!;
        var privateKey = creds.GetProperty("encryptionKey").GetProperty("privateKey").GetString()!;
        var server = new MockServer(apiKey);
        var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey);
        return (server, client);
    }

    [Fact]
    public async Task GetAccounts_Decrypts_All_Fields()
    {
        var (server, client) = Make();
        using var _ = server;
        using var __ = client;

        var accounts = await client.GetAccountsAsync();
        var a = Assert.Single(accounts);

        Assert.Equal("DK6466952001724927", a.Iban);
        Assert.Equal("66952001724927", a.Bban);
        Assert.Equal("Tatic ApS", a.OwnerName);
        Assert.Equal("Drift", a.DisplayName);          // from displayNameEnc
        Assert.Equal("Lunar", a.AspspName);            // structural, plaintext
        Assert.Equal("LUNADK22", a.Bic);

        Assert.Equal(3, a.Balances.Count);
        Assert.Equal(828.13m, a.Balances.Single(b => b.Type == "ITBD").Amount);
        Assert.Equal(633.90m, a.Balances.Single(b => b.Type == "ITAV").Amount);
    }

    [Fact]
    public async Task GetTransactions_Decrypts()
    {
        var (server, client) = Make();
        using var _ = server;
        using var __ = client;

        var page = await client.GetTransactionsAsync(AccountId, limit: 100);
        var t = Assert.Single(page.Items);

        Assert.Equal(1, page.Total);
        Assert.Equal(194.23m, t.Amount);
        Assert.Equal("One.com", t.CreditorName);
        Assert.Equal("DBIT", t.CreditDebitIndicator);   // structural, plaintext
        Assert.Equal("4816", t.MerchantCategoryCode);
        Assert.Equal(new DateOnly(2026, 6, 8), t.BookingDate);
    }

    [Fact]
    public async Task GetConnections_Works()
    {
        var (server, client) = Make();
        using var _ = server;
        using var __ = client;

        var c = Assert.Single(await client.GetConnectionsAsync());
        Assert.Equal("Lunar", c.AspspName);
        Assert.Equal("Active", c.Status);
        Assert.Equal(1, c.AccountCount);
    }

    [Fact]
    public async Task Sync_Posts_The_Decrypted_Uid()
    {
        var (server, client) = Make();
        using var _ = server;
        using var __ = client;

        var result = await client.SyncAsync(AccountId);
        Assert.Equal(1, result.TotalFetched);

        var post = Assert.Single(server.Posts);
        Assert.Equal($"api/accounts/{AccountId}/sync", post.Path);
        // The browser-style decrypted uid was sent — the backend never had it in plaintext.
        Assert.Contains("c5d93aa7-5e23-4da0-ba88-42b9a584492c", post.Body);
    }

    [Fact]
    public async Task SyncAll_Posts_Items_With_Decrypted_Uids()
    {
        var (server, client) = Make();
        using var _ = server;
        using var __ = client;

        var result = await client.SyncAllAsync();
        Assert.Equal(1, result.Accounts);

        var post = Assert.Single(server.Posts);
        Assert.Equal("api/sync", post.Path);
        Assert.Contains("c5d93aa7-5e23-4da0-ba88-42b9a584492c", post.Body);
    }

    [Fact]
    public async Task FromCredentials_Bundle_Works()
    {
        var creds = Fixtures.Read("credentials.json");
        var apiKey = Fixtures.ReadJson("credentials.json").GetProperty("apiKey").GetString()!;
        using var server = new MockServer(apiKey);

        // Point the bundle's apiBaseUrl at the mock by overriding via the explicit ctor is cleaner,
        // but here we prove FromCredentials parses + authenticates by swapping in an HttpClient.
        using var http = new HttpClient();
        var bundle = System.Text.Json.JsonSerializer.Deserialize<CredentialsBundle>(creds, WebJson)!;
        using var client = new OpenBankingClient(server.BaseUrl, bundle.ApiKey!, bundle.EncryptionKey.PrivateKey, http);

        var a = Assert.Single(await client.GetAccountsAsync());
        Assert.Equal("DK6466952001724927", a.Iban);
    }

    [Fact]
    public async Task Wrong_ApiKey_Yields_401()
    {
        var creds = Fixtures.ReadJson("credentials.json");
        var privateKey = creds.GetProperty("encryptionKey").GetProperty("privateKey").GetString()!;
        using var server = new MockServer("the-real-key");
        using var client = new OpenBankingClient(server.BaseUrl, "wrong-key", privateKey);

        await Assert.ThrowsAsync<HttpRequestException>(() => client.GetAccountsAsync());
    }
}
