/** Error returned by the API as RFC 9457 problem details. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly retryAfterSeconds?: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

type Method = 'GET' | 'POST' | 'PATCH';

/**
 * Calls the same-origin API. Session cookies are HttpOnly and scoped by the
 * server; the SPA never sees or stores credentials.
 */
export async function request<T>(method: Method, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 204) return undefined as T;
  const data: unknown = await res.json().catch(() => ({}));
  if (!res.ok) throw toApiError(res, data);
  return data as T;
}

function toApiError(res: Response, data: unknown): ApiError {
  const problem = (data ?? {}) as { code?: string; title?: string };
  const retry = Number(res.headers.get('Retry-After')) || undefined;
  return new ApiError(
    res.status,
    problem.code ?? 'unknown',
    problem.title ?? 'Request failed',
    retry,
  );
}

/** Human-readable message for any thrown value. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  return 'Something went wrong. Check your connection and try again.';
}
