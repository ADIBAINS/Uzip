import { NextRequest } from "next/server";
import { sessionIdentity, sessionResponse } from "../../server/session";
import { upstream } from "../../server/upstream";

export async function GET(request: NextRequest) {
  try {
    const identity = await sessionIdentity(request);
    if (!identity) return sessionResponse({ error: "Sign in required." }, 401);
    const response = await upstream("control", "/api/v1/session", identity);
    if (response.ok) return sessionResponse(await response.json());
    if (response.status === 401) return sessionResponse({ error: "Sign in required." }, 401);
  } catch {
    // The client can retry a temporary failure without discarding its session.
  }
  return sessionResponse({ error: "Session is temporarily unavailable." }, 503);
}
