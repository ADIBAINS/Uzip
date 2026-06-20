import { getToken } from "./auth";

export const CONTROL_API = process.env.NEXT_PUBLIC_CONTROL_API ?? "http://localhost:8080";
export const ANALYTICS_API = process.env.NEXT_PUBLIC_ANALYTICS_API ?? "http://localhost:8083";

const devHeaders: Record<string, string> = process.env.NEXT_PUBLIC_DEV_AUTH !== "false" ? {
  "X-User-ID": "00000000-0000-0000-0000-000000000001",
  "X-Tenant-ID": "00000000-0000-0000-0000-000000000001",
  "X-Role": "owner",
} : {};

export async function api<T>(base: string, path: string, init?: RequestInit): Promise<T> {
  const token = await getToken();
  const headers = new Headers(init?.headers);
  headers.set("Content-Type", "application/json");
  Object.entries(devHeaders).forEach(([key, value]) => headers.set(key, value));
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(`${base}${path}`, { ...init, headers });
  if (response.status === 401 && process.env.NEXT_PUBLIC_DEV_AUTH === "false") {
    localStorage.removeItem("uzip_id_token");
    localStorage.removeItem("uzip_refresh_token");
  }
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new Error(body?.error?.message ?? `Request failed (${response.status})`);
  }
  if (response.status === 204) return undefined as T;
  return response.json();
}
