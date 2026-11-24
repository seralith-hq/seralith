const TOKEN_KEY = "sigil.token";

export function captureToken(hash: string, storage: Storage = sessionStorage): string | null {
  const match = /(?:^#|&)token=([A-Za-z0-9_-]+)/.exec(hash);
  if (match?.[1]) {
    storage.setItem(TOKEN_KEY, match[1]);
    return match[1];
  }
  return storage.getItem(TOKEN_KEY);
}

export function token(): string {
  return sessionStorage.getItem(TOKEN_KEY) ?? "";
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token()}`);
  if (init.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const res = await fetch(`/api/v1${path}`, { ...init, headers });
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const body: unknown = text ? JSON.parse(text) : undefined;
  if (!res.ok) {
    const message =
      body && typeof body === "object" && "error" in body ? String(body.error) : `HTTP ${res.status}`;
    throw new ApiError(res.status, message);
  }
  return body as T;
}

export const post = <T>(path: string, body: unknown) =>
  api<T>(path, { method: "POST", body: JSON.stringify(body) });

export const del = (path: string) => api<undefined>(path, { method: "DELETE" });

export function wsURL(path: string): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}/api/v1${path}?token=${encodeURIComponent(token())}`;
}

export function eventsURL(): string {
  return `/api/v1/events?token=${encodeURIComponent(token())}`;
}
