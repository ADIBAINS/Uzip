"use client";

import { useEffect, useState } from "react";
import { completeLogin } from "../../auth";

export default function Callback() {
  const [message, setMessage] = useState("Completing sign in…");
  useEffect(() => {
    const query = new URLSearchParams(window.location.search);
    const error = query.get("error_description") ?? query.get("error");
    const code = query.get("code");
    const state = query.get("state") ?? "";
    if (error) { setMessage(error); return; }
    if (!code) { setMessage("Cognito did not return an authorization code."); return; }
    completeLogin(code, state)
      .then(() => window.location.replace("/"))
      .catch((reason) => setMessage(reason instanceof Error ? reason.message : "Sign in failed."));
  }, []);
  return <section className="panel"><h2>Authentication</h2><p>{message}</p></section>;
}
