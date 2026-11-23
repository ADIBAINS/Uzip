"use client";

import { createContext, useContext, useEffect, useMemo, useState } from "react";

type Claims = {
  sub: string;
  email?: string;
  "custom:tenant_id"?: string;
  "custom:role"?: string;
  exp: number;
};

type AuthState = {
  ready: boolean;
  authenticated: boolean;
  claims?: Claims;
  login: () => Promise<void>;
  logout: () => void;
};

const AuthContext = createContext<AuthState | null>(null);
const DEV_AUTH = process.env.NEXT_PUBLIC_DEV_AUTH !== "false";
const DEV_SESSION_KEY = "uzip_dev_session";
const DOMAIN = process.env.NEXT_PUBLIC_COGNITO_DOMAIN ?? "";
const CLIENT_ID = process.env.NEXT_PUBLIC_COGNITO_CLIENT_ID ?? "";
const REDIRECT_URI = process.env.NEXT_PUBLIC_COGNITO_REDIRECT_URI ?? "http://localhost:3000/auth/callback";
const LOGOUT_URI = process.env.NEXT_PUBLIC_COGNITO_LOGOUT_URI ?? "http://localhost:3000";

function decode(token: string): Claims | undefined {
  try {
    return JSON.parse(atob(token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/")));
  } catch {
    return undefined;
  }
}

function base64url(bytes: Uint8Array) {
  return btoa(String.fromCharCode(...bytes)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function storedToken() {
  if (typeof window === "undefined" || DEV_AUTH) return "";
  const token = localStorage.getItem("uzip_id_token") ?? "";
  const claims = decode(token);
  if (!claims || claims.exp * 1000 <= Date.now()) {
    localStorage.removeItem("uzip_id_token");
    return "";
  }
  return token;
}

export async function getToken() {
  const current = storedToken();
  if (current || DEV_AUTH) return current;
  const refreshToken = localStorage.getItem("uzip_refresh_token");
  if (!refreshToken) return "";
  const response = await fetch(`${DOMAIN}/oauth2/token`, {
    method: "POST",
    signal: AbortSignal.timeout(15000),
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "refresh_token",
      client_id: CLIENT_ID,
      refresh_token: refreshToken,
    }),
  });
  if (!response.ok) {
    localStorage.removeItem("uzip_refresh_token");
    return "";
  }
  const tokens = await response.json();
  localStorage.setItem("uzip_id_token", tokens.id_token);
  return tokens.id_token as string;
}

export async function completeLogin(code: string, returnedState: string) {
  const verifier = sessionStorage.getItem("uzip_pkce_verifier");
  const expectedState = sessionStorage.getItem("uzip_oauth_state");
  if (!verifier) throw new Error("Login session expired. Start again.");
  if (!expectedState || expectedState !== returnedState) throw new Error("Login state validation failed.");
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    client_id: CLIENT_ID,
    code,
    redirect_uri: REDIRECT_URI,
    code_verifier: verifier,
  });
  const response = await fetch(`${DOMAIN}/oauth2/token`, {
    method: "POST",
    signal: AbortSignal.timeout(15000),
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });
  if (!response.ok) throw new Error("Cognito token exchange failed.");
  const tokens = await response.json();
  localStorage.setItem("uzip_id_token", tokens.id_token);
  if (tokens.refresh_token) localStorage.setItem("uzip_refresh_token", tokens.refresh_token);
  sessionStorage.removeItem("uzip_pkce_verifier");
  sessionStorage.removeItem("uzip_oauth_state");
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [ready, setReady] = useState(false);
  const [claims, setClaims] = useState<Claims>();
  const [devAuthenticated, setDevAuthenticated] = useState(false);
  useEffect(() => {
    if (DEV_AUTH) {
      setDevAuthenticated(localStorage.getItem(DEV_SESSION_KEY) !== "signed-out");
      setReady(true);
      return;
    }
    getToken()
      .then((token) => { if (token) setClaims(decode(token)); })
      .catch(() => setClaims(undefined))
      .finally(() => setReady(true));
  }, []);

  async function login() {
    if (DEV_AUTH) {
      localStorage.setItem(DEV_SESSION_KEY, "signed-in");
      setDevAuthenticated(true);
      return;
    }
    if (!DOMAIN || !CLIENT_ID) throw new Error("Cognito dashboard configuration is incomplete.");
    const verifier = base64url(crypto.getRandomValues(new Uint8Array(32)));
    const state = base64url(crypto.getRandomValues(new Uint8Array(24)));
    const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
    const challenge = base64url(new Uint8Array(digest));
    sessionStorage.setItem("uzip_pkce_verifier", verifier);
    sessionStorage.setItem("uzip_oauth_state", state);
    const query = new URLSearchParams({
      client_id: CLIENT_ID,
      response_type: "code",
      scope: "openid email profile",
      redirect_uri: REDIRECT_URI,
      code_challenge_method: "S256",
      code_challenge: challenge,
      state,
    });
    window.location.assign(`${DOMAIN}/oauth2/authorize?${query}`);
  }

  function logout() {
    localStorage.removeItem("uzip_id_token");
    localStorage.removeItem("uzip_refresh_token");
    if (DEV_AUTH) {
      localStorage.setItem(DEV_SESSION_KEY, "signed-out");
      setDevAuthenticated(false);
      return;
    }
    const query = new URLSearchParams({ client_id: CLIENT_ID, logout_uri: LOGOUT_URI });
    window.location.assign(`${DOMAIN}/logout?${query}`);
  }

  const value = useMemo(() => ({
    ready,
    authenticated: DEV_AUTH ? devAuthenticated : Boolean(claims),
    claims,
    login,
    logout,
  }), [ready, claims, devAuthenticated]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthProvider");
  return value;
}

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const auth = useAuth();
  const [error, setError] = useState("");
  if (!auth.ready) return <p className="empty">Loading session…</p>;
  if (!auth.authenticated) return <section className="panel auth-panel"><p className="eyebrow">Secure workspace</p><h2>Sign in required</h2><p>{DEV_AUTH ? "Enter the demo workspace to continue." : "Use your organization account to continue."}</p>{error && <p role="alert" className="error">{error}</p>}<button onClick={() => auth.login().catch(e => setError(e.message))}>{DEV_AUTH ? "Enter demo workspace" : "Sign in with Cognito"}</button></section>;
  return children;
}
