/**
 * The one error shape (design section 7, "HTTP"):
 *
 *     {"error": {"code": "snake_case_code", "message": "Human readable"}}
 */

export interface ErrorBody {
  error: { code: string; message: string; [key: string]: unknown };
}

export class HttpError extends Error {
  readonly statusCode: number;
  readonly code: string;
  readonly details: Record<string, unknown> | undefined;

  constructor(
    statusCode: number,
    code: string,
    message: string,
    details?: Record<string, unknown>,
  ) {
    super(message);
    this.name = 'HttpError';
    this.statusCode = statusCode;
    this.code = code;
    this.details = details;
  }

  toBody(): ErrorBody {
    return { error: { code: this.code, message: this.message, ...(this.details ?? {}) } };
  }
}

export function errorBody(
  code: string,
  message: string,
  details?: Record<string, unknown>,
): ErrorBody {
  return { error: { code, message, ...(details ?? {}) } };
}

export const badRequest = (code: string, message: string, details?: Record<string, unknown>) =>
  new HttpError(400, code, message, details);
export const unauthorized = (message = 'Missing or invalid credentials') =>
  new HttpError(401, 'unauthorized', message);
export const forbidden = (message = 'Insufficient role') =>
  new HttpError(403, 'forbidden', message);
export const notFound = (code: string, message: string) => new HttpError(404, code, message);
export const conflict = (code: string, message: string, details?: Record<string, unknown>) =>
  new HttpError(409, code, message, details);
export const unavailable = (code: string, message: string, details?: Record<string, unknown>) =>
  new HttpError(503, code, message, details);
