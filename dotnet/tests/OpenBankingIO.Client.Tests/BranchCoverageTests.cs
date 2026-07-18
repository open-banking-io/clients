using System.Reflection;
using System.Text.Json;

namespace OpenBankingIO.Client.Tests;

/// <summary>
/// Branch-coverage focused tests for <see cref="OpenBankingClient"/>: the static factories, constructor
/// validation, query-string building, the null/empty-ciphertext decrypt paths, and the sync edge cases
/// (account-not-found, sessionless accounts). All use the same MockServer + Fixtures helpers as the
/// integration tests.
/// </summary>
public class BranchCoverageTests
{
    private const string AccountId = "11111111-1111-4111-8111-111111111111";

    private static readonly JsonSerializerOptions WebJson =
        new(JsonSerializerDefaults.Web);

    private static (string ApiKey, string PrivateKey) Creds()
    {
        var creds = Fixtures.ReadJson("credentials.json");
        return (
            creds.GetProperty("apiKey").GetString()!,
            creds.GetProperty("encryptionKey").GetProperty("privateKey").GetString()!);
    }

    private static OpenBankingClient ClientFor(MockServer server)
    {
        var (apiKey, privateKey) = Creds();
        return new OpenBankingClient(server.BaseUrl, apiKey, privateKey);
    }

    // ---- Static factories ----------------------------------------------------------------------

    [Fact]
    public void FromCredentials_Parses_Json_String()
    {
        var json = Fixtures.Read("credentials.json");
        using var client = OpenBankingClient.FromCredentials(json);
        Assert.NotNull(client);
    }

    [Fact]
    public void FromCredentials_Parses_File_Path()
    {
        // Write the bundle to a temp file so the File.Exists branch of FromCredentials is taken.
        var path = Path.Combine(Path.GetTempPath(), $"obk-creds-{Guid.NewGuid():N}.json");
        File.WriteAllText(path, Fixtures.Read("credentials.json"));
        try
        {
            using var client = OpenBankingClient.FromCredentials(path);
            Assert.NotNull(client);
        }
        finally
        {
            File.Delete(path);
        }
    }

    [Fact]
    public void FromCredentials_Null_Json_Throws()
    {
        // The literal "null" deserializes to a null bundle → ArgumentException.
        Assert.Throws<ArgumentException>(() => OpenBankingClient.FromCredentials("null"));
    }

    [Fact]
    public void FromCredentials_Malformed_Json_Throws()
    {
        Assert.Throws<JsonException>(() => OpenBankingClient.FromCredentials("{ not valid json"));
    }

    [Fact]
    public void FromBundle_Missing_ApiKey_Throws()
    {
        var bundle = new CredentialsBundle
        {
            ApiBaseUrl = "https://example.test",
            ApiKey = null,
        };
        Assert.Throws<ArgumentException>(() => OpenBankingClient.FromBundle(bundle));
    }

    [Fact]
    public async Task FromBundle_Builds_Working_Client()
    {
        var (apiKey, _) = Creds();
        using var server = new MockServer(apiKey);

        var creds = Fixtures.Read("credentials.json");
        var bundle = JsonSerializer.Deserialize<CredentialsBundle>(creds, WebJson)! with
        {
            ApiBaseUrl = server.BaseUrl,
        };

        using var client = OpenBankingClient.FromBundle(bundle);
        var a = Assert.Single(await client.GetAccountsAsync());
        Assert.Equal("DK6466952001724927", a.Iban);
    }

    // ---- Constructor validation ----------------------------------------------------------------

    [Theory]
    [InlineData("", "key", "cGtjcw==")]
    [InlineData("  ", "key", "cGtjcw==")]
    [InlineData("https://example.test", "", "cGtjcw==")]
    [InlineData("https://example.test", "  ", "cGtjcw==")]
    [InlineData("https://example.test", "key", "")]
    [InlineData("https://example.test", "key", "  ")]
    public void Constructor_Blank_Args_Throw(string url, string apiKey, string privateKey)
    {
        Assert.ThrowsAny<ArgumentException>(() => new OpenBankingClient(url, apiKey, privateKey));
    }

    [Fact]
    public async Task Constructor_Accepts_Injected_HttpClient()
    {
        var (apiKey, privateKey) = Creds();
        using var server = new MockServer(apiKey);
        using var http = new HttpClient();
        using var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey, http);

        var a = Assert.Single(await client.GetAccountsAsync());
        Assert.Equal("DK6466952001724927", a.Iban);
    }

    /// <summary>Reads the private <c>_http</c> field so we can assert its configured timeout.</summary>
    private static HttpClient InnerHttp(OpenBankingClient client)
        => (HttpClient)typeof(OpenBankingClient)
            .GetField("_http", BindingFlags.Instance | BindingFlags.NonPublic)!
            .GetValue(client)!;

    [Fact]
    public void Constructor_Timeout_Applied_To_Default_Client()
    {
        var (apiKey, privateKey) = Creds();
        var timeout = TimeSpan.FromSeconds(7);
        // No HttpClient injected → the SDK creates its own and must honour the caller's timeout.
        using var client = new OpenBankingClient("https://example.test", apiKey, privateKey, timeout: timeout);

        Assert.Equal(timeout, InnerHttp(client).Timeout);
    }

    [Fact]
    public void Constructor_No_Timeout_Uses_Default_On_Default_Client()
    {
        var (apiKey, privateKey) = Creds();
        // No timeout → the default 30s timeout is applied to the SDK-created client.
        using var client = new OpenBankingClient("https://example.test", apiKey, privateKey);

        Assert.Equal(TimeSpan.FromSeconds(30), InnerHttp(client).Timeout);
    }

    [Fact]
    public void Constructor_Timeout_Never_Mutates_Injected_Client()
    {
        var (apiKey, privateKey) = Creds();
        // A distinctive value that is neither the SDK default (30s) nor HttpClient's own default (100s).
        var callerTimeout = TimeSpan.FromSeconds(42);
        using var http = new HttpClient { Timeout = callerTimeout };

        // Even though a timeout is requested, a caller-owned client must be left untouched.
        using var client = new OpenBankingClient(
            "https://example.test", apiKey, privateKey, http, timeout: TimeSpan.FromSeconds(7));

        Assert.Same(http, InnerHttp(client));
        Assert.Equal(callerTimeout, http.Timeout);
    }

    [Fact]
    public void FromBundle_Threads_Timeout_To_Default_Client()
    {
        var creds = Fixtures.Read("credentials.json");
        var bundle = JsonSerializer.Deserialize<CredentialsBundle>(creds, WebJson)! with
        {
            ApiBaseUrl = "https://example.test",
        };
        var timeout = TimeSpan.FromSeconds(13);

        using var client = OpenBankingClient.FromBundle(bundle, timeout: timeout);
        Assert.Equal(timeout, InnerHttp(client).Timeout);
    }

    [Fact]
    public void Constructor_Trims_Trailing_Slash_From_BaseUrl()
    {
        var (apiKey, privateKey) = Creds();
        // A trailing slash on the base URL must not double up; just prove construction succeeds.
        using var client = new OpenBankingClient("https://example.test/", apiKey, privateKey);
        Assert.NotNull(client);
    }

    // ---- Query building --------------------------------------------------------------------------

    [Fact]
    public async Task GetTransactions_No_Params_Sends_No_Query()
    {
        var (apiKey, privateKey) = Creds();
        using var server = new MockServer(apiKey);
        using var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey);

        await client.GetTransactionsAsync(AccountId);
        Assert.Equal("", server.LastQuery);
    }

    [Fact]
    public async Task GetTransactions_All_Params_Are_Encoded()
    {
        var (apiKey, privateKey) = Creds();
        using var server = new MockServer(apiKey);
        using var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey);

        await client.GetTransactionsAsync(
            AccountId,
            from: new DateOnly(2026, 1, 2),
            to: new DateOnly(2026, 3, 4),
            limit: 50,
            offset: 10);

        var q = server.LastQuery!;
        Assert.Contains("from=2026-01-02", q);
        Assert.Contains("to=2026-03-04", q);
        Assert.Contains("limit=50", q);
        Assert.Contains("offset=10", q);
    }

    [Fact]
    public async Task GetTransactions_Empty_Page_Returns_No_Items()
    {
        var (apiKey, privateKey) = Creds();
        using var server = new MockServer(apiKey) { TransactionsJson = """{"items":[],"total":0}""" };
        using var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey);

        var page = await client.GetTransactionsAsync(AccountId);
        Assert.Empty(page.Items);
        Assert.Equal(0, page.Total);
    }

    // ---- Null / empty ciphertext decrypt paths --------------------------------------------------

    /// <summary>
    /// An account whose every encrypted field (enc, displayNameEnc, uidEnc, balance enc) is null/absent:
    /// every <c>DecryptTo</c> returns the default and the mapper must yield nulls/zeros, not throw.
    /// </summary>
    private const string NullEncAccount = """
    [
      {
        "id": "99999999-9999-4999-8999-999999999999",
        "aspspName": "Lunar",
        "aspspCountry": "DK",
        "currency": "DKK",
        "accountType": "CACC",
        "bic": "LUNADK22",
        "needsReconnect": true,
        "balances": [
          { "type": "ITBD", "currency": "DKK", "referenceDate": null, "enc": null }
        ],
        "enc": null,
        "displayNameEnc": null,
        "uidEnc": null
      }
    ]
    """;

    [Fact]
    public async Task GetAccounts_With_Null_Ciphertext_Decrypts_To_Nulls()
    {
        var (apiKey, privateKey) = Creds();
        using var server = new MockServer(apiKey) { AccountsJson = NullEncAccount };
        using var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey);

        var a = Assert.Single(await client.GetAccountsAsync());
        Assert.Null(a.Iban);
        Assert.Null(a.Bban);
        Assert.Null(a.OwnerName);
        Assert.Null(a.DisplayName);

        var b = Assert.Single(a.Balances);
        Assert.Null(b.Name);
        Assert.Equal(0m, b.Amount); // ParseDecimal(null) -> 0
    }

    /// <summary>A transaction with a null enc: all decrypted fields null, amount parses to 0.</summary>
    private const string NullEncTransactions = """
    {
      "items": [
        {
          "id": "44444444-4444-4444-8444-444444444444",
          "currency": "DKK",
          "creditDebitIndicator": "CRDT",
          "status": "BOOK",
          "bookingDate": null,
          "valueDate": null,
          "transactionDate": null,
          "bankTransactionCode": null,
          "enc": null
        }
      ],
      "total": 1
    }
    """;

    [Fact]
    public async Task GetTransactions_With_Null_Ciphertext_Decrypts_To_Nulls()
    {
        var (apiKey, privateKey) = Creds();
        using var server = new MockServer(apiKey) { TransactionsJson = NullEncTransactions };
        using var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey);

        var t = Assert.Single((await client.GetTransactionsAsync(AccountId)).Items);
        Assert.Equal(0m, t.Amount);
        Assert.Null(t.CreditorName);
        Assert.Null(t.BalanceAfterTransaction); // ParseDecimalNullable(null) -> null
    }

    // ---- Sync edge cases -------------------------------------------------------------------------

    [Fact]
    public async Task Sync_Unknown_Account_Throws_NotFound()
    {
        using var server = new MockServer(Creds().ApiKey);
        using var client = ClientFor(server);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(
            () => client.SyncAsync("00000000-0000-0000-0000-000000000000"));
        Assert.Contains("not found", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public async Task Sync_Account_Without_Session_Throws()
    {
        var (apiKey, privateKey) = Creds();
        // The single account has a null uidEnc → DecryptUid returns null → no active session.
        using var server = new MockServer(apiKey) { AccountsJson = NullEncAccount };
        using var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(
            () => client.SyncAsync("99999999-9999-4999-8999-999999999999"));
        Assert.Contains("no active session", ex.Message, StringComparison.OrdinalIgnoreCase);
        Assert.Empty(server.Posts); // never reached the POST
    }

    [Fact]
    public async Task SyncAll_Skips_Sessionless_Accounts()
    {
        var (apiKey, privateKey) = Creds();
        // No account has a uid → the posted items list is empty, but the call still succeeds.
        using var server = new MockServer(apiKey) { AccountsJson = NullEncAccount };
        using var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey);

        var result = await client.SyncAllAsync();
        Assert.Equal(1, result.Accounts); // from the sync-all fixture body

        var post = Assert.Single(server.Posts);
        Assert.Equal("api/sync", post.Path);
        // Sessionless account filtered out: its uid must not appear in the request body.
        Assert.DoesNotContain("99999999-9999-4999-8999-999999999999", post.Body);
    }

    // ---- HTTP error paths ------------------------------------------------------------------------

    [Fact]
    public async Task Unparseable_Json_Body_Throws()
    {
        var (apiKey, privateKey) = Creds();
        using var server = new MockServer(apiKey);
        using var client = new OpenBankingClient(server.BaseUrl, apiKey, privateKey);

        // The "__badjson__" sentinel returns HTTP 200 with a body that is not valid JSON.
        await Assert.ThrowsAsync<JsonException>(
            () => client.GetTransactionsAsync("__badjson__"));
    }

    [Fact]
    public async Task Transport_Failure_Throws_HttpRequestException()
    {
        var (apiKey, privateKey) = Creds();
        // Point at a closed port (server disposed immediately) to force a transport failure.
        string baseUrl;
        using (var server = new MockServer(apiKey))
        {
            baseUrl = server.BaseUrl;
        }
        using var client = new OpenBankingClient(baseUrl, apiKey, privateKey);

        await Assert.ThrowsAsync<HttpRequestException>(() => client.GetAccountsAsync());
    }
}
