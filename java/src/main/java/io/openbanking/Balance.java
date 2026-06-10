package io.openbanking;

import java.math.BigDecimal;

/** A balance snapshot. {@code type} is the ISO 20022 code (ITBD booked, ITAV available, ...). */
public record Balance(
    String type, String name, BigDecimal amount, String currency, String referenceDate) {}
