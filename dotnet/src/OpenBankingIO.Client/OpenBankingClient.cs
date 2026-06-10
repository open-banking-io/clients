using System.Globalization;
using System.Net.Http.Json;
using System.Security.Cryptography;
using System.Text.Json;

namespace OpenBankingIO.Client;

/// <summary>
/// Server-to-server client for open-banking.io. Authenticates with an API key (<c>X-Api-Key</c>) and
/// decrypts the zero-knowledge data envelopes locally with the exported private key — the service only
/// ever returns ciphertext it cannot read.
/// </summary>
public sealed class OpenBankingClient : IDisposable
{
    private readonly HttpClient _http;
    private readonly ECDiffieHellman _privateKey;
    private readonly bool _ownsHttp;

    /// <param name="apiBaseUrl">e.g. <c>https://open-banking.io</c>.</param>
    /// <param name="apiKey">The API key from your credentials bundle.</param>
    /// <param name="privateKeyPkcs8Base64">The base64 PKCS#8 encryption private key from your bundle.</param>
    /// <param name="httpClient">Optional injected client (e.g. for testing); otherwise one is created.</param>
    public OpenBankingClient(string apiBaseUrl, string apiKey, string privateKeyPkcs8Base64, HttpClient? httpClient = null)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(apiBaseUrl);
        ArgumentException.ThrowIfNullOrWhiteSpace(apiKey);
        ArgumentException.ThrowIfNullOrWhiteSpace(privateKeyPkcs8Base64);

        _privateKey = ECDiffieHellman.Create();
        _privateKey.ImportPkcs8PrivateKey(Convert.FromBase64String(privateKeyPkcs8Base64), out _);

        _ownsHttp = httpClient is null;
        _http = httpClient ?? new HttpClient();
        _http.BaseAddress = new Uri(apiBaseUrl.TrimEnd('/') + "/");
        _http.DefaultRequestHeaders.Remove("X-Api-Key");
        _http.DefaultRequestHeaders.Add("X-Api-Key", apiKey);
    }

    /// <summary>Builds a client from a credentials-bundle JSON string or a path to a bundle file.</summary>
    public static OpenBankingClient FromCredentials(string jsonOrPath, HttpClient? httpClient = null)
    {
        var json = File.Exists(jsonOrPath) ? File.ReadAllText(jsonOrPath) : jsonOrPath;
        var bundle = JsonSerializer.Deserialize<CredentialsBundle>(json, Json.Options)
            ?? throw new ArgumentException("Could not parse the credentials bundle", nameof(jsonOrPath));
        return FromBundle(bundle, httpClient);
    }

    public static OpenBankingClient FromBundle(CredentialsBundle bundle, HttpClient? httpClient = null)
    {
        if (string.IsNullOrWhiteSpace(bundle.ApiKey))
            throw new ArgumentException("The credentials bundle has no apiKey", nameof(bundle));
        return new OpenBankingClient(bundle.ApiBaseUrl, bundle.ApiKey!, bundle.EncryptionKey.PrivateKey, httpClient);
    }

    /// <summary>Lists the user's accounts with all sensitive fields decrypted.</summary>
    public async Task<IReadOnlyList<Account>> GetAccountsAsync(CancellationToken ct = default)
    {
        var wires = await GetAccountWiresAsync(ct).ConfigureAwait(false);
        return wires.Select(MapAccount).ToList();
    }

    /// <summary>Returns a page of an account's statement, newest first, with decrypted fields.</summary>
    public async Task<TransactionPage> GetTransactionsAsync(
        string accountId, DateOnly? from = null, DateOnly? to = null, int? limit = null, int? offset = null,
        CancellationToken ct = default)
    {
        var query = new List<string>();
        if (from is { } f) query.Add($"from={f:yyyy-MM-dd}");
        if (to is { } t) query.Add($"to={t:yyyy-MM-dd}");
        if (limit is { } l) query.Add($"limit={l}");
        if (offset is { } o) query.Add($"offset={o}");
        var qs = query.Count > 0 ? "?" + string.Join("&", query) : "";

        var page = await _http.GetFromJsonAsync<TransactionPageWire>(
            $"api/accounts/{accountId}/transactions{qs}", Json.Options, ct).ConfigureAwait(false)
            ?? new TransactionPageWire();

        return new TransactionPage { Items = page.Items.Select(MapTransaction).ToList(), Total = page.Total };
    }

    /// <summary>Lists the user's bank connections.</summary>
    public async Task<IReadOnlyList<Connection>> GetConnectionsAsync(CancellationToken ct = default)
    {
        var wires = await _http.GetFromJsonAsync<List<ConnectionWire>>("api/connections", Json.Options, ct)
            .ConfigureAwait(false) ?? [];
        return wires.Select(c => new Connection
        {
            SessionId = c.SessionId,
            AspspName = c.AspspName,
            AspspCountry = c.AspspCountry,
            ValidUntil = c.ValidUntil,
            Status = c.Status,
            AccountCount = c.AccountCount,
            LastSyncedAt = c.LastSyncedAt,
            PsuType = c.PsuType,
        }).ToList();
    }

    /// <summary>
    /// Triggers an online sync of one account: decrypts that account's Enable Banking uid and posts it,
    /// so the service can fetch fresh data without ever holding the uid in plaintext.
    /// </summary>
    public async Task<SyncResult> SyncAsync(string accountId, CancellationToken ct = default)
    {
        var wires = await GetAccountWiresAsync(ct).ConfigureAwait(false);
        var account = wires.FirstOrDefault(a => a.Id == accountId)
            ?? throw new InvalidOperationException($"Account {accountId} not found");
        var uid = DecryptUid(account)
            ?? throw new InvalidOperationException("Account has no active session (reconnect required) — cannot sync");

        var response = await _http.PostAsJsonAsync($"api/accounts/{accountId}/sync", new { uid }, Json.Options, ct)
            .ConfigureAwait(false);
        response.EnsureSuccessStatusCode();
        var result = await response.Content.ReadFromJsonAsync<SyncResultWire>(Json.Options, ct).ConfigureAwait(false)
            ?? new SyncResultWire(0, 0);
        return new SyncResult(result.NewTransactions, result.TotalFetched);
    }

    /// <summary>Triggers an online sync of every account that has an active session.</summary>
    public async Task<SyncAllResult> SyncAllAsync(CancellationToken ct = default)
    {
        var wires = await GetAccountWiresAsync(ct).ConfigureAwait(false);
        var items = wires
            .Select(a => (a.Id, Uid: DecryptUid(a)))
            .Where(x => x.Uid is not null)
            .Select(x => new { accountId = x.Id, uid = x.Uid })
            .ToList();

        var response = await _http.PostAsJsonAsync("api/sync", new { items }, Json.Options, ct).ConfigureAwait(false);
        response.EnsureSuccessStatusCode();
        var result = await response.Content.ReadFromJsonAsync<SyncAllResultWire>(Json.Options, ct).ConfigureAwait(false)
            ?? new SyncAllResultWire(0, 0);
        return new SyncAllResult(result.Accounts, result.NewTransactions);
    }

    private async Task<List<AccountWire>> GetAccountWiresAsync(CancellationToken ct)
        => await _http.GetFromJsonAsync<List<AccountWire>>("api/accounts", Json.Options, ct).ConfigureAwait(false) ?? [];

    private string? DecryptUid(AccountWire a)
        => Envelope.DecryptTo<UidEnc>(_privateKey, a.UidEnc)?.Uid;

    private Account MapAccount(AccountWire a)
    {
        var acc = Envelope.DecryptTo<AccountEnc>(_privateKey, a.Enc);
        var name = Envelope.DecryptTo<DisplayNameEnc>(_privateKey, a.DisplayNameEnc);

        return new Account
        {
            Id = a.Id,
            AspspName = a.AspspName,
            AspspCountry = a.AspspCountry,
            Currency = a.Currency,
            AccountType = a.AccountType,
            Bic = a.Bic,
            NeedsReconnect = a.NeedsReconnect,
            Iban = acc?.Iban,
            Bban = acc?.Bban,
            OwnerName = acc?.OwnerName,
            AccountName = acc?.AccountName,
            Product = acc?.Product,
            DisplayName = name?.DisplayName,
            Balances = a.Balances.Select(b =>
            {
                var dec = Envelope.DecryptTo<BalanceEnc>(_privateKey, b.Enc);
                return new Balance
                {
                    Type = b.Type,
                    Currency = b.Currency,
                    ReferenceDate = b.ReferenceDate,
                    Name = dec?.Name,
                    Amount = ParseDecimal(dec?.Amount),
                };
            }).ToList(),
        };
    }

    private Transaction MapTransaction(TransactionWire t)
    {
        var d = Envelope.DecryptTo<TransactionEnc>(_privateKey, t.Enc);
        return new Transaction
        {
            Id = t.Id,
            Currency = t.Currency,
            CreditDebitIndicator = t.CreditDebitIndicator,
            Status = t.Status,
            BookingDate = t.BookingDate,
            ValueDate = t.ValueDate,
            TransactionDate = t.TransactionDate,
            BankTransactionCode = t.BankTransactionCode,
            Amount = ParseDecimal(d?.Amount),
            CreditorName = d?.CreditorName,
            CreditorIban = d?.CreditorIban,
            CreditorBban = d?.CreditorBban,
            CreditorAgentBic = d?.CreditorAgentBic,
            DebtorName = d?.DebtorName,
            DebtorIban = d?.DebtorIban,
            DebtorBban = d?.DebtorBban,
            RemittanceInformation = d?.RemittanceInformation,
            Note = d?.Note,
            ReferenceNumber = d?.ReferenceNumber,
            ExchangeRate = d?.ExchangeRate,
            MerchantCategoryCode = d?.MerchantCategoryCode,
            BalanceAfterCurrency = d?.BalanceAfterCurrency,
            BalanceAfterTransaction = ParseDecimalNullable(d?.BalanceAfter),
        };
    }

    private static decimal ParseDecimal(string? s)
        => string.IsNullOrEmpty(s) ? 0m : decimal.Parse(s, CultureInfo.InvariantCulture);

    private static decimal? ParseDecimalNullable(string? s)
        => string.IsNullOrEmpty(s) ? null : decimal.Parse(s, CultureInfo.InvariantCulture);

    public void Dispose()
    {
        _privateKey.Dispose();
        if (_ownsHttp) _http.Dispose();
    }
}
