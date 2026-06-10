using System.Net;
using System.Net.Sockets;
using System.Text;

namespace OpenBankingIO.Client.Tests;

/// <summary>
/// A tiny local HTTP server that serves the encrypted fixture responses and enforces the
/// <c>X-Api-Key</c> header — a realistic mock of the open-banking.io API for integration tests.
/// </summary>
internal sealed class MockServer : IDisposable
{
    private readonly HttpListener _listener = new();
    private readonly string _apiKey;
    private readonly Task _loop;

    public string BaseUrl { get; }
    public List<(string Path, string Body)> Posts { get; } = [];

    /// <summary>The <c>User-Agent</c> header seen on the most recent request, if any.</summary>
    public string? LastUserAgent { get; private set; }

    /// <summary>The raw query string (without the leading '?') of the most recent request, if any.</summary>
    public string? LastQuery { get; private set; }

    /// <summary>Overrides the JSON body returned for <c>GET api/accounts</c> (otherwise the fixture is served).</summary>
    public string? AccountsJson { get; set; }

    /// <summary>Overrides the JSON body returned for a transactions GET (otherwise the fixture is served).</summary>
    public string? TransactionsJson { get; set; }

    public MockServer(string apiKey)
    {
        _apiKey = apiKey;
        var port = FreePort();
        BaseUrl = $"http://127.0.0.1:{port}/";
        _listener.Prefixes.Add(BaseUrl);
        _listener.Start();
        _loop = Task.Run(LoopAsync);
    }

    private async Task LoopAsync()
    {
        while (_listener.IsListening)
        {
            HttpListenerContext ctx;
            try { ctx = await _listener.GetContextAsync(); }
            catch { break; }
            _ = HandleAsync(ctx);
        }
    }

    private async Task HandleAsync(HttpListenerContext ctx)
    {
        var req = ctx.Request;
        var res = ctx.Response;
        try
        {
            LastUserAgent = req.Headers["User-Agent"];
            LastQuery = req.Url!.Query.TrimStart('?');

            if (req.Headers["X-Api-Key"] != _apiKey)
            {
                res.StatusCode = 401;
                res.Close();
                return;
            }

            var path = req.Url!.AbsolutePath.TrimStart('/');
            var method = req.HttpMethod;

            // Sentinel for the non-2xx negative test: the "__error__" account id → 500.
            if (path.Contains("__error__", StringComparison.Ordinal))
            {
                res.StatusCode = 500;
                res.Close();
                return;
            }

            // Sentinel for the unparseable-body test: any path containing "__badjson__" → 200 with junk.
            if (path.Contains("__badjson__", StringComparison.Ordinal))
            {
                await WriteBodyAsync(res, "{ this is not valid json");
                return;
            }

            // body is either a literal JSON string (override) or a "fixture:<path>" marker resolved below.
            var body = (method, path) switch
            {
                ("GET", "api/accounts") => AccountsJson ?? "fixture:api/accounts.json",
                ("GET", "api/connections") => "fixture:api/connections.json",
                ("GET", _) when path.StartsWith("api/accounts/", StringComparison.Ordinal) && path.EndsWith("/transactions", StringComparison.Ordinal) => TransactionsJson ?? "fixture:api/transactions.json",
                ("POST", "api/sync") => RecordAndReturn(req, path, "fixture:api/sync-all.json"),
                ("POST", _) when path.StartsWith("api/accounts/", StringComparison.Ordinal) && path.EndsWith("/sync", StringComparison.Ordinal) => RecordAndReturn(req, path, "fixture:api/sync.json"),
                _ => null,
            };

            if (body is null)
            {
                res.StatusCode = 404;
                res.Close();
                return;
            }

            if (body.StartsWith("fixture:", StringComparison.Ordinal))
                body = Fixtures.Read(body["fixture:".Length..]);

            await WriteBodyAsync(res, body);
        }
        catch
        {
            try { res.StatusCode = 500; res.Close(); } catch { /* ignore */ }
        }
    }

    private static async Task WriteBodyAsync(HttpListenerResponse res, string body)
    {
        try
        {
            var bytes = Encoding.UTF8.GetBytes(body);
            res.ContentType = "application/json";
            res.ContentLength64 = bytes.Length;
            await res.OutputStream.WriteAsync(bytes);
            res.Close();
        }
        catch
        {
            try { res.StatusCode = 500; res.Close(); } catch { /* ignore */ }
        }
    }

    private string RecordAndReturn(HttpListenerRequest req, string path, string fixture)
    {
        using var reader = new StreamReader(req.InputStream, req.ContentEncoding);
        Posts.Add((path, reader.ReadToEnd()));
        return fixture;
    }

    private static int FreePort()
    {
        var l = new TcpListener(IPAddress.Loopback, 0);
        l.Start();
        var port = ((IPEndPoint)l.LocalEndpoint).Port;
        l.Stop();
        return port;
    }

    public void Dispose()
    {
        try { _listener.Stop(); } catch { /* ignore */ }
        _listener.Close();
    }
}
