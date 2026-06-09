namespace OpenBankingIO.Client;

// ---- Public, decrypted models -------------------------------------------------------------------

/// <summary>A bank account with its sensitive fields decrypted.</summary>
public sealed record Account
{
    public required string Id { get; init; }
    public required string AspspName { get; init; }
    public required string AspspCountry { get; init; }
    public required string Currency { get; init; }
    public string? AccountType { get; init; }
    public string? Bic { get; init; }
    public bool NeedsReconnect { get; init; }

    // Decrypted from the account envelope.
    public string? Iban { get; init; }
    public string? Bban { get; init; }
    public string? OwnerName { get; init; }
    public string? AccountName { get; init; }
    public string? Product { get; init; }
    public string? DisplayName { get; init; }

    public IReadOnlyList<Balance> Balances { get; init; } = [];
}

/// <summary>A balance snapshot. <see cref="Type"/> is the ISO 20022 code (ITBD booked, ITAV available, …).</summary>
public sealed record Balance
{
    public required string Type { get; init; }
    public string? Name { get; init; }
    public decimal Amount { get; init; }
    public required string Currency { get; init; }
    public DateOnly? ReferenceDate { get; init; }
}

/// <summary>A statement transaction with its sensitive fields decrypted.</summary>
public sealed record Transaction
{
    public required string Id { get; init; }
    public required string Currency { get; init; }
    public required string CreditDebitIndicator { get; init; }
    public string? Status { get; init; }
    public DateOnly? BookingDate { get; init; }
    public DateOnly? ValueDate { get; init; }
    public DateOnly? TransactionDate { get; init; }
    public string? BankTransactionCode { get; init; }

    public decimal Amount { get; init; }
    public string? CreditorName { get; init; }
    public string? CreditorIban { get; init; }
    public string? CreditorBban { get; init; }
    public string? CreditorAgentBic { get; init; }
    public string? DebtorName { get; init; }
    public string? DebtorIban { get; init; }
    public string? DebtorBban { get; init; }
    public string? RemittanceInformation { get; init; }
    public string? Note { get; init; }
    public string? ReferenceNumber { get; init; }
    public string? ExchangeRate { get; init; }
    public string? MerchantCategoryCode { get; init; }
    public decimal? BalanceAfterTransaction { get; init; }
    public string? BalanceAfterCurrency { get; init; }
}

/// <summary>A page of transactions, newest first.</summary>
public sealed record TransactionPage
{
    public required IReadOnlyList<Transaction> Items { get; init; }
    public int Total { get; init; }
}

/// <summary>A bank connection (consent). The id is an opaque handle for reconnect/disconnect.</summary>
public sealed record Connection
{
    public required string SessionId { get; init; }
    public required string AspspName { get; init; }
    public required string AspspCountry { get; init; }
    public DateTimeOffset ValidUntil { get; init; }
    public required string Status { get; init; }
    public int AccountCount { get; init; }
    public DateTimeOffset? LastSyncedAt { get; init; }
    public string? PsuType { get; init; }
}

public sealed record SyncResult(int NewTransactions, int TotalFetched);

public sealed record SyncAllResult(int Accounts, int NewTransactions);

// ---- Internal wire DTOs (what the API returns; sensitive fields are ciphertext) -----------------

internal sealed record AccountWire
{
    public string Id { get; init; } = "";
    public string AspspName { get; init; } = "";
    public string AspspCountry { get; init; } = "";
    public string Currency { get; init; } = "";
    public string? AccountType { get; init; }
    public string? Bic { get; init; }
    public bool NeedsReconnect { get; init; }
    public List<BalanceWire> Balances { get; init; } = [];
    public string? Enc { get; init; }
    public string? DisplayNameEnc { get; init; }
    public string? UidEnc { get; init; }
}

internal sealed record BalanceWire
{
    public string Type { get; init; } = "";
    public string Currency { get; init; } = "";
    public DateOnly? ReferenceDate { get; init; }
    public string? Enc { get; init; }
}

internal sealed record TransactionPageWire
{
    public List<TransactionWire> Items { get; init; } = [];
    public int Total { get; init; }
}

internal sealed record TransactionWire
{
    public string Id { get; init; } = "";
    public string Currency { get; init; } = "";
    public string CreditDebitIndicator { get; init; } = "";
    public string? Status { get; init; }
    public DateOnly? BookingDate { get; init; }
    public DateOnly? ValueDate { get; init; }
    public DateOnly? TransactionDate { get; init; }
    public string? BankTransactionCode { get; init; }
    public string? Enc { get; init; }
}

internal sealed record ConnectionWire
{
    public string SessionId { get; init; } = "";
    public string AspspName { get; init; } = "";
    public string AspspCountry { get; init; } = "";
    public DateTimeOffset ValidUntil { get; init; }
    public string Status { get; init; } = "";
    public int AccountCount { get; init; }
    public DateTimeOffset? LastSyncedAt { get; init; }
    public string? PsuType { get; init; }
}

internal sealed record SyncResultWire(int NewTransactions, int TotalFetched);
internal sealed record SyncAllResultWire(int Accounts, int NewTransactions);

// ---- Decrypted envelope payloads (the camelCase contract with the backend) ----------------------

internal sealed record AccountEnc(string? OwnerName, string? Iban, string? Bban, string? AccountName, string? Product);
internal sealed record DisplayNameEnc(string? DisplayName);
internal sealed record UidEnc(string? Uid);
internal sealed record BalanceEnc(string? Amount, string? Name);

internal sealed record TransactionEnc(
    string? Amount, string? CreditorName, string? CreditorIban, string? CreditorBban, string? CreditorAgentBic,
    string? DebtorName, string? DebtorIban, string? DebtorBban, string? DebtorAgentBic,
    string? RemittanceInformation, string? Note, string? ReferenceNumber, string? ExchangeRate,
    string? MerchantCategoryCode, string? BalanceAfter, string? BalanceAfterCurrency, string? RawJson);
