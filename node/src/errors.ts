import type { SyncFailureReason } from "./models.js";

/** A sync the service refused. `reason` is the stable code to branch on. */
export class SyncError extends Error {
  readonly status: number;
  readonly reason: SyncFailureReason | null;
  readonly bankErrorCode: string | null;
  readonly retryAfterSeconds: number | null;

  constructor(
    message: string,
    status: number,
    reason: SyncFailureReason | null,
    bankErrorCode: string | null = null,
    retryAfterSeconds: number | null = null,
  ) {
    super(message);
    this.name = "SyncError";
    this.status = status;
    this.reason = reason;
    this.bankErrorCode = bankErrorCode;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}
