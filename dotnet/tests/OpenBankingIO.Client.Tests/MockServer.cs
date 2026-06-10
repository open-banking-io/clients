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

            var fixture = (method, path) switch
            {
                ("GET", "api/accounts") => "api/accounts.json",
                ("GET", "api/connections") => "api/connections.json",
                ("GET", _) when path.StartsWith("api/accounts/", StringComparison.Ordinal) && path.EndsWith("/transactions", StringComparison.Ordinal) => "api/transactions.json",
                ("POST", "api/sync") => RecordAndReturn(req, path, "api/sync-all.json"),
                ("POST", _) when path.StartsWith("api/accounts/", StringComparison.Ordinal) && path.EndsWith("/sync", StringComparison.Ordinal) => RecordAndReturn(req, path, "api/sync.json"),
                _ => null,
            };

            if (fixture is null)
            {
                res.StatusCode = 404;
                res.Close();
                return;
            }

            var bytes = Encoding.UTF8.GetBytes(Fixtures.Read(fixture));
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
