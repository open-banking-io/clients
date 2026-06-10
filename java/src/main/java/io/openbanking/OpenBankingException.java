package io.openbanking;

/** Unchecked error raised by the open-banking.io client (config, crypto, or HTTP failures). */
public class OpenBankingException extends RuntimeException {
    public OpenBankingException(String message) {
        super(message);
    }

    public OpenBankingException(String message, Throwable cause) {
        super(message, cause);
    }
}
