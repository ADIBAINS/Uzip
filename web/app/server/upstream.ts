export const demoMode = process.env.NEXT_PUBLIC_DEV_AUTH !== "false";

const services = {
  control: process.env.CONTROL_API_URL || process.env.NEXT_PUBLIC_CONTROL_API || "http://localhost:8080",
  analytics: process.env.ANALYTICS_API_URL || process.env.NEXT_PUBLIC_ANALYTICS_API || "http://localhost:8083",
};

const demoID = "00000000-0000-0000-0000-000000000001";

export function upstream(service: keyof typeof services, path: string, identity: string, init?: RequestInit) {
  const headers = new Headers(init?.headers);
  if (demoMode) {
    headers.set("X-User-ID", demoID);
    headers.set("X-Tenant-ID", demoID);
    headers.set("X-Role", "owner");
    headers.set("X-Email", "demo@uzip.local");
  } else {
    headers.set("Authorization", `Bearer ${identity}`);
  }
  return fetch(`${services[service]}${path}`, {
    ...init, headers, cache: "no-store", redirect: "error", signal: AbortSignal.timeout(8000),
  });
}
