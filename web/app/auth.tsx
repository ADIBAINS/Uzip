"use client";

import { createContext, useContext, useEffect, useState } from "react";

type Claims = {
  sub: string;
  email?: string;
  "custom:tenant_id"?: string;
  "custom:role"?: string;
};

type AuthState = {
  ready: boolean;
  authenticated: boolean;
  claims?: Claims;
  error: string;
  login: () => Promise<void>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthState | null>(null);
const DEV_AUTH = process.env.NEXT_PUBLIC_DEV_AUTH !== "false";

async function sessionAction(action: "login" | "logout") {
  const response = await fetch(`/api/session/${action}`, { method: "POST", signal: AbortSignal.timeout(10000) });
  const result = await response.json();
  if (!response.ok) throw new Error(result.error);
  window.location.assign(result.url);
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [ready, setReady] = useState(false);
  const [claims, setClaims] = useState<Claims>();
  const [error, setError] = useState("");

  useEffect(() => {
    // Remove credentials written by the previous browser token flow.
    try {
      localStorage.removeItem("uzip_id_token");
      localStorage.removeItem("uzip_refresh_token");
    } catch {
      // Browser storage may be disabled.
    }
    const expired = () => setClaims(undefined);
    window.addEventListener("uzip:session-expired", expired);
    const controller = new AbortController();
    const params = new URLSearchParams(window.location.search);
    if (params.get("sign-in") === "failed") {
      setError("Sign-in failed or your workspace access is missing. Try signing in again.");
    }
    fetch("/api/session", { signal: AbortSignal.any([controller.signal, AbortSignal.timeout(10000)]) })
      .then(async response => {
        if (response.ok) setClaims(await response.json());
        else if (response.status !== 401) throw new Error("Session is temporarily unavailable.");
      })
      .catch(() => { if (!controller.signal.aborted) setError("Session is temporarily unavailable."); })
      .finally(() => { if (!controller.signal.aborted) setReady(true); });
    return () => {
      controller.abort();
      window.removeEventListener("uzip:session-expired", expired);
    };
  }, []);

  async function login() {
    setError("");
    try { await sessionAction("login"); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Sign-in failed."); }
  }

  async function logout() {
    setError("");
    try { await sessionAction("logout"); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Sign-out failed."); }
  }

  return <AuthContext.Provider value={{ ready, authenticated: Boolean(claims), claims, error, login, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthProvider");
  return value;
}

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const auth = useAuth();
  if (!auth.ready) return <p className="empty">Loading session…</p>;
  if (!auth.authenticated) return <section className="panel auth-panel"><p className="eyebrow">Secure workspace</p><h2>Sign in required</h2><p>{DEV_AUTH ? "Enter the demo workspace to continue." : "Use your organization account to continue."}</p><button onClick={auth.login}>{DEV_AUTH ? "Enter demo workspace" : "Sign in with Cognito"}</button></section>;
  return children;
}
