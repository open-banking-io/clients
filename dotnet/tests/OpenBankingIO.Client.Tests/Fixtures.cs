using System.Text.Json;

namespace OpenBankingIO.Client.Tests;

/// <summary>Reads the shared, language-agnostic fixtures copied next to the test assembly.</summary>
internal static class Fixtures
{
    private static readonly string Dir = Path.Combine(AppContext.BaseDirectory, "fixtures");

    public static string Read(string relative) => File.ReadAllText(Path.Combine(Dir, relative.Replace('/', Path.DirectorySeparatorChar)));

    public static JsonElement ReadJson(string relative) => JsonDocument.Parse(Read(relative)).RootElement;
}
