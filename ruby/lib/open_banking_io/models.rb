# frozen_string_literal: true

module OpenBankingIO
  # Public, decrypted models for the open-banking.io client.
  #
  # These are immutable keyword-initialised value objects (Struct). Amounts are exposed
  # as BigDecimal; dates as String (ISO-8601) as returned by the service.

  # A balance snapshot. +type+ is the ISO 20022 code (ITBD booked, ITAV available, ...).
  Balance = Struct.new(
    :type,
    :name,
    :amount,          # BigDecimal
    :currency,
    :reference_date,
    keyword_init: true
  )

  # A bank account with its sensitive fields decrypted.
  Account = Struct.new(
    :id,
    :aspsp_name,
    :aspsp_country,
    :currency,
    :account_type,
    :bic,
    :needs_reconnect,
    :iban,
    :bban,
    :owner_name,
    :account_name,
    :product,
    :display_name,
    :balances,        # Array<Balance>
    keyword_init: true
  )

  # A statement transaction with its sensitive fields decrypted.
  Transaction = Struct.new(
    :id,
    :currency,
    :credit_debit_indicator,
    :status,
    :booking_date,
    :value_date,
    :transaction_date,
    :bank_transaction_code,
    :amount,          # BigDecimal
    :creditor_name,
    :creditor_iban,
    :creditor_bban,
    :creditor_agent_bic,
    :debtor_name,
    :debtor_iban,
    :debtor_bban,
    :debtor_agent_bic,
    :remittance_information,
    :note,
    :reference_number,
    :exchange_rate,
    :merchant_category_code,
    :balance_after_transaction,   # BigDecimal or nil
    :balance_after_currency,
    keyword_init: true
  )

  # A page of transactions, newest first.
  TransactionPage = Struct.new(:items, :total, keyword_init: true)

  # A bank connection (consent).
  Connection = Struct.new(
    :session_id,
    :aspsp_name,
    :aspsp_country,
    :valid_until,
    :status,
    :account_count,
    :last_synced_at,
    :psu_type,
    keyword_init: true
  )

  SyncResult = Struct.new(:new_transactions, :total_fetched, keyword_init: true)

  SyncAllResult = Struct.new(:accounts, :new_transactions, keyword_init: true)
end
