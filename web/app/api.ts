export async function graphql<T>(query: string, variables: Record<string, unknown> = {}, signal?: AbortSignal): Promise<T> {
  const timeout = AbortSignal.timeout(15000);
  let response: Response;
  try {
    response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query, variables }),
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    });
  } catch {
    throw new Error("Unable to reach the service. Check your connection and try again.");
  }
  const result = await response.json().catch(() => null);
  const error = result?.errors?.[0];
  if (response.status === 401 || error?.extensions?.status === 401) {
    window.dispatchEvent(new Event("uzip:session-expired"));
  }
  if (!response.ok || error || !result?.data) {
    throw new Error(error?.message ?? `Request failed (${response.status})`);
  }
  return result.data;
}
