// Thin fetch wrapper: JSON, CSRF header, auth redirect, typed errors.

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

let onUnauthorized: (() => void) | null = null;

export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

interface Options {
  method?: string;
  body?: unknown;
  /** Value for the X-Confirm header on dangerous endpoints. */
  confirm?: string;
}

export async function api<T>(path: string, opts: Options = {}): Promise<T> {
  const headers: Record<string, string> = { "X-CP": "1" };
  let body: string | undefined;
  if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(opts.body);
  }
  if (opts.confirm !== undefined) {
    headers["X-Confirm"] = opts.confirm;
  }
  const res = await fetch(path, {
    method: opts.method ?? "GET",
    headers,
    body,
    credentials: "same-origin",
  });
  if (res.status === 401) {
    onUnauthorized?.();
    throw new ApiError(401, "authentication required");
  }
  const text = await res.text();
  let data: unknown = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    // non-JSON error body
  }
  if (!res.ok) {
    const msg =
      data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : res.statusText;
    throw new ApiError(res.status, msg);
  }
  return data as T;
}
