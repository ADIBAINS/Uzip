import { createHash, randomBytes, timingSafeEqual } from "node:crypto";
import { setTimeout as wait } from "node:timers/promises";
import { NextRequest, NextResponse } from "next/server";
import { createClient } from "@redis/client";
import { demoMode, upstream } from "./upstream";

const callback = process.env.NEXT_PUBLIC_COGNITO_REDIRECT_URI || "http://localhost:3000/auth/callback";
const logoutURL = process.env.NEXT_PUBLIC_COGNITO_LOGOUT_URI || "http://localhost:3000";
const cognito = process.env.NEXT_PUBLIC_COGNITO_DOMAIN;
const clientID = process.env.NEXT_PUBLIC_COGNITO_CLIENT_ID;
const sessionTTL = demoMode ? 60 * 60 : 7 * 24 * 60 * 60;
const idPattern = /^[A-Za-z0-9_-]{43}$/;

type Tokens = { id_token: string; refresh_token?: string; expires_in: number };
type Session = { identity: string; refresh?: string; refreshAt?: number };

let connection: ReturnType<typeof connect> | undefined;

async function connect() {
  const redis = createClient({
    url: process.env.REDIS_URL || "redis://localhost:6379/0",
    disableOfflineQueue: true,
    socket: { connectTimeout: 2000, reconnectStrategy: retries => retries < 3 ? 250 : false },
  });
  redis.on("error", () => { if (!redis.isOpen) connection = undefined; });
  await redis.connect();
  return redis;
}

function redisClient() {
  if (!connection) connection = connect().catch(error => { connection = undefined; throw error; });
  return connection;
}

function origin(request: NextRequest) {
  return process.env.UZIP_DASHBOARD_ORIGIN || (demoMode ? request.nextUrl.origin : new URL(callback).origin);
}

export function sameOrigin(request: NextRequest) {
  return request.headers.get("origin") === origin(request);
}

function cookieOptions(request: NextRequest, maxAge: number) {
  return { httpOnly: true, sameSite: "lax" as const, secure: new URL(origin(request)).protocol === "https:", path: "/", maxAge };
}

function sessionCookie(request: NextRequest) {
  return cookieOptions(request, 0).secure ? "__Host-uzip_session" : "uzip_session";
}

const loginCookie = "uzip_oauth";
const keyFor = (id: string) => `browser-session:${createHash("sha256").update(id).digest("hex")}`;

async function exchange(parameters: Record<string, string>): Promise<Tokens | null> {
  if (!cognito || !clientID) throw new Error("Cognito login is not configured.");
  const response = await fetch(new URL("/oauth2/token", cognito), {
    method: "POST", cache: "no-store", redirect: "error", signal: AbortSignal.timeout(5000),
    body: new URLSearchParams({ client_id: clientID, ...parameters }),
  });
  if (response.status === 400 || response.status === 401) return null;
  if (!response.ok) throw new Error("Cognito token exchange failed.");
  const tokens = await response.json() as Partial<Tokens>;
  if (!tokens.id_token || typeof tokens.expires_in !== "number" || tokens.expires_in <= 0) {
    throw new Error("Cognito returned an invalid token response.");
  }
  return tokens as Tokens;
}

function tokenSession(tokens: Tokens, refresh?: string): Session {
  return {
    identity: tokens.id_token,
    refresh: tokens.refresh_token ?? refresh,
    refreshAt: Date.now() + Math.max(1, tokens.expires_in - 20) * 1000,
  };
}

async function establish(request: NextRequest, response: NextResponse, session: Session) {
  const redis = await redisClient();
  const oldID = request.cookies.get(sessionCookie(request))?.value;
  const id = randomBytes(32).toString("base64url");
  if (oldID && idPattern.test(oldID)) await redis.del(keyFor(oldID));
  await redis.set(keyFor(id), JSON.stringify(session), { expiration: { type: "EX", value: sessionTTL } });
  response.cookies.set(sessionCookie(request), id, cookieOptions(request, sessionTTL));
  return response;
}

export async function beginLogin(request: NextRequest) {
  if (!sameOrigin(request)) return sessionResponse({ error: "Invalid origin." }, 403);
  if (demoMode) {
    if (await sessionIdentity(request)) return sessionResponse({ url: "/" });
    const redis = await redisClient();
    const attempts = await redis.eval(`local count=redis.call('INCR',KEYS[1])
      if count==1 then redis.call('EXPIRE',KEYS[1],60) end
      return count`, { keys: ["browser-session:demo-logins"], arguments: [] });
    if (Number(attempts) > 120) return sessionResponse({ error: "The demo is busy. Try again in a minute." }, 429);
    return establish(request, sessionResponse({ url: "/" }), { identity: "demo" });
  }
  if (!cognito || !clientID) return sessionResponse({ error: "Cognito login is not configured." }, 503);

  const state = randomBytes(32).toString("base64url");
  const verifier = randomBytes(32).toString("base64url");
  const location = new URL("/oauth2/authorize", cognito);
  location.search = new URLSearchParams({
    client_id: clientID, response_type: "code", scope: "openid email profile", redirect_uri: callback,
    state, code_challenge_method: "S256", code_challenge: createHash("sha256").update(verifier).digest("base64url"),
  }).toString();
  const response = sessionResponse({ url: location.href });
  response.cookies.set(loginCookie, `${state}.${verifier}`, { ...cookieOptions(request, 600), path: "/auth/callback" });
  return response;
}

export async function finishLogin(request: NextRequest) {
  const [expected, verifier] = request.cookies.get(loginCookie)?.value.split(".") ?? [];
  const returned = request.nextUrl.searchParams.get("state") ?? "";
  const code = request.nextUrl.searchParams.get("code");
  const validState = expected && verifier && idPattern.test(expected) && idPattern.test(returned) &&
    idPattern.test(verifier) && timingSafeEqual(Buffer.from(expected), Buffer.from(returned));
  let response = NextResponse.redirect(new URL("/?sign-in=failed", origin(request)), 303);
  response.headers.set("Cache-Control", "no-store");
  if (!demoMode && code && validState && code.length <= 2048) {
    try {
      const tokens = await exchange({ grant_type: "authorization_code", redirect_uri: callback, code, code_verifier: verifier });
      if (tokens && (await upstream("control", "/api/v1/session", tokens.id_token)).ok) {
        response = await establish(request, NextResponse.redirect(new URL("/", origin(request)), 303), tokenSession(tokens));
        response.headers.set("Cache-Control", "no-store");
      }
    } catch (error) {
      console.error("Could not complete Cognito sign-in", error instanceof Error ? error.message : error);
    }
  }
  response.cookies.set(loginCookie, "", { ...cookieOptions(request, 0), path: "/auth/callback" });
  return response;
}

export async function sessionIdentity(request: NextRequest): Promise<string | null> {
  const id = request.cookies.get(sessionCookie(request))?.value;
  if (!id || !idPattern.test(id)) return null;
  const redis = await redisClient();
  const key = keyFor(id);
  const refreshLock = `${key}:refresh`;
  const owner = randomBytes(16).toString("base64url");
  for (let attempt = 0; attempt < 60; attempt++) {
    const stored = await redis.get(key);
    if (!stored) return null;
    const session = JSON.parse(stored) as Session;
    if (session.refreshAt === undefined || Date.now() < session.refreshAt) return session.identity;
    if (!session.refresh) return null;
    if (!(await redis.set(refreshLock, owner, { condition: "NX", expiration: { type: "EX", value: 8 } }))) {
      await wait(100);
      continue;
    }
    try {
      if (stored !== await redis.get(key)) continue;
      const tokens = await exchange({ grant_type: "refresh_token", refresh_token: session.refresh });
      if (!tokens) {
        await redis.del(key);
        return null;
      }
      const next = tokenSession(tokens, session.refresh);
      const updated = await redis.set(key, JSON.stringify(next), { condition: "XX", expiration: "KEEPTTL" });
      return updated ? next.identity : null;
    } finally {
      await redis.eval("if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0",
        { keys: [refreshLock], arguments: [owner] });
    }
  }
  throw new Error("Session refresh is temporarily unavailable.");
}

export async function endSession(request: NextRequest) {
  if (!sameOrigin(request)) return sessionResponse({ error: "Invalid origin." }, 403);
  const cookie = sessionCookie(request);
  const id = request.cookies.get(cookie)?.value;
  if (id && idPattern.test(id)) {
    const redis = await redisClient();
    await redis.del(keyFor(id));
  }
  const location = !demoMode && cognito && clientID ? new URL("/logout", cognito) : null;
  if (location && clientID) location.search = new URLSearchParams({ client_id: clientID, logout_uri: logoutURL }).toString();
  const response = sessionResponse({ url: location?.href ?? "/" });
  response.cookies.set(cookie, "", cookieOptions(request, 0));
  return response;
}

export function sessionResponse(data: unknown, status = 200) {
  return NextResponse.json(data, { status, headers: { "Cache-Control": "no-store" } });
}
